package ninerouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatCompletionsForwardsPayloadAndAuth(t *testing.T) {
	var (
		seenPath   string
		seenAuth   string
		seenBody   []byte
		seenMethod string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		seenMethod = r.Method
		seenBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	client, err := New(upstream.URL+"/v1", "router-key")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	response, err := client.ChatCompletions(context.Background(), []byte(`{"model":"x"}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	defer response.Body.Close()

	if seenMethod != http.MethodPost {
		t.Fatalf("method = %s", seenMethod)
	}
	if seenPath != "/v1/chat/completions" {
		t.Fatalf("path = %s", seenPath)
	}
	if seenAuth != "Bearer router-key" {
		t.Fatalf("authorization = %q", seenAuth)
	}
	if string(seenBody) != `{"model":"x"}` {
		t.Fatalf("body = %s", seenBody)
	}
}

func TestBaseURLWithoutVersionGetsV1(t *testing.T) {
	var seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer upstream.Close()

	client, err := New(upstream.URL, "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.ListModels(context.Background()); err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if seenPath != "/v1/models" {
		t.Fatalf("path = %s, want /v1/models", seenPath)
	}
}

func TestListModelsParsesCatalog(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"nvidia/a"},{"id":" nvidia/b "},{"id":""}]}`))
	}))
	defer upstream.Close()

	client, err := New(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ids, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if len(ids) != 2 || ids[0] != "nvidia/a" || ids[1] != "nvidia/b" {
		t.Fatalf("ids = %#v", ids)
	}
}

func TestListModelsReturnsStatusErrorWithoutRetry(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer upstream.Close()

	client, err := New(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	targetCalls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetCalls++
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/steal", http.StatusFound)
	}))
	defer upstream.Close()

	client, err := New(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	response, err := client.ChatCompletions(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("ChatCompletions() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", response.StatusCode)
	}
	if targetCalls != 0 {
		t.Fatalf("redirect target was called %d times", targetCalls)
	}
}

func TestNormalizeEndpointBaseRejectsUnsafeURLs(t *testing.T) {
	for _, raw := range []string{
		"ftp://localhost:20128/v1",
		"http://user:pass@localhost:20128/v1",
		"http://localhost:20128/v1?x=1",
		"http://localhost:20128/v1#fragment",
	} {
		if _, err := New(raw, ""); err == nil {
			t.Fatalf("New(%q) succeeded, want error", raw)
		}
	}
}

func TestUpstreamErrorJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer upstream.Close()

	client, err := New(upstream.URL+"/v1", "")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = client.ListModels(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	var upstreamError *UpstreamError
	if !errors.As(err, &upstreamError) {
		t.Fatalf("error type = %T", err)
	}
	if upstreamError.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", upstreamError.StatusCode)
	}
}
