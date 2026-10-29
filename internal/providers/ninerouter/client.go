package ninerouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const userAgent = "deepsi-llm-api-gateway/0.1.0"

// Client is the only outbound AI client used by the gateway. It talks to the
// configured 9Router deployment and never connects to an AI provider directly.
type Client struct {
	endpointBase string
	apiKey       string
	httpClient   *http.Client
}

func New(baseURL, apiKey string) (*Client, error) {
	base, err := normalizeEndpointBase(baseURL)
	if err != nil {
		return nil, err
	}

	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
	}

	return &Client{
		endpointBase: base,
		apiKey:       strings.TrimSpace(apiKey),
		httpClient: &http.Client{
			Transport: transport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *Client) ChatCompletions(ctx context.Context, body []byte) (*http.Response, error) {
	return c.post(ctx, "/chat/completions", body)
}

func (c *Client) Embeddings(ctx context.Context, body []byte) (*http.Response, error) {
	return c.post(ctx, "/embeddings", body)
}

func (c *Client) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointBase+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("User-Agent", userAgent)
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return c.httpClient.Do(request)
}

// ListModels discovers upstream model identifiers for validation and
// diagnostics. It never returns the raw body or upstream URL to API clients.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpointBase+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build upstream models request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	if c.apiKey != "" {
		request.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("reach 9Router: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read 9Router models response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &UpstreamError{StatusCode: response.StatusCode}
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode 9Router models response: %w", err)
	}

	ids := make([]string, 0, len(payload.Data))
	for _, model := range payload.Data {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (c *Client) CloseIdleConnections() {
	c.httpClient.CloseIdleConnections()
}

// UpstreamError is returned when 9Router responds with a non-success status for
// a discovery request.
type UpstreamError struct {
	StatusCode int
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("9Router returned HTTP %d", e.StatusCode)
}

func normalizeEndpointBase(raw string) (string, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid 9Router base URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid 9Router base URL: scheme must be http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("invalid 9Router base URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" {
		path = "/v1"
	}
	parsed.Path = path
	parsed.RawPath = ""
	return parsed.String(), nil
}

// SanitizeHTTPError converts an arbitrary transport error into a short,
// non-sensitive category. Details belong in server-side logs, not responses.
func SanitizeHTTPError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) {
		return "request_canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream_timeout"
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return "upstream_timeout"
	}
	return "upstream_unreachable"
}
