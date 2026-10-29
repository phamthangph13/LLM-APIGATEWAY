package gateway

import (
	"log/slog"
	"net/http"

	"github.com/deepsi/llm-api-gateway/internal/config"
	"github.com/deepsi/llm-api-gateway/internal/httpx"
	"github.com/deepsi/llm-api-gateway/internal/models"
	"github.com/deepsi/llm-api-gateway/internal/providers/ninerouter"
)

type Gateway struct {
	cfg       *config.Config
	registry  *models.Registry
	upstream  *ninerouter.Client
	log       *slog.Logger
	sanitizer textSanitizer
}

func New(cfg *config.Config, registry *models.Registry, upstream *ninerouter.Client, log *slog.Logger) *Gateway {
	return &Gateway{
		cfg:       cfg,
		registry:  registry,
		upstream:  upstream,
		log:       log,
		sanitizer: newTextSanitizer(cfg, registry),
	}
}

// Models serves GET /v1/models using only public DeepSI identifiers.
func (g *Gateway) Models(w http.ResponseWriter, r *http.Request) {
	list := g.registry.List()
	data := make([]map[string]any, 0, len(list))
	for _, model := range list {
		data = append(data, map[string]any{
			"id":       model.ID,
			"object":   "model",
			"created":  model.Created,
			"owned_by": "deepsi",
		})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"object": "list",
		"data":   data,
	})
}
