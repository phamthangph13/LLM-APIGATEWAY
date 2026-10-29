package models

import (
	"fmt"
	"strings"
)

type Kind string

const (
	KindChat       Kind = "chat"
	KindEmbeddings Kind = "embeddings"
)

// Model describes a public DeepSI model and the upstream identifier it maps to.
type Model struct {
	ID           string
	UpstreamID   string
	Kind         Kind
	Description  string
	Created      int64
	SystemPrompt string
}

// Registry is the single source of truth for public model identifiers,
// aliases, upstream mappings, and server-managed identity instructions.
type Registry struct {
	ordered    []Model
	byAlias    map[string]int
	byUpstream map[string]int
}

func NewRegistry(m1, s1, v1, e1 string) (*Registry, error) {
	definitions := []Model{
		{
			ID:           "DeepSI-M1",
			UpstreamID:   strings.TrimSpace(m1),
			Kind:         KindChat,
			Description:  "Fast and versatile chat, coding, reasoning, and multimodal model",
			Created:      1767225600,
			SystemPrompt: PromptM1,
		},
		{
			ID:           "DeepSI-S1",
			UpstreamID:   strings.TrimSpace(s1),
			Kind:         KindChat,
			Description:  "Advanced reasoning and complex problem-solving model",
			Created:      1767225600,
			SystemPrompt: PromptS1,
		},
		{
			ID:           "DeepSI-V1",
			UpstreamID:   strings.TrimSpace(v1),
			Kind:         KindChat,
			Description:  "Omni and multimodal reasoning model",
			Created:      1767225600,
			SystemPrompt: PromptV1,
		},
		{
			ID:          "DeepSI-E1",
			UpstreamID:  strings.TrimSpace(e1),
			Kind:        KindEmbeddings,
			Description: "Text and document embedding model",
			Created:     1767225600,
		},
	}

	registry := &Registry{
		ordered:    make([]Model, 0, len(definitions)),
		byAlias:    make(map[string]int, len(definitions)),
		byUpstream: make(map[string]int, len(definitions)),
	}

	for _, definition := range definitions {
		if definition.UpstreamID == "" {
			return nil, fmt.Errorf("model %s has an empty upstream identifier", definition.ID)
		}
		alias := normalizeAlias(definition.ID)
		if _, exists := registry.byAlias[alias]; exists {
			return nil, fmt.Errorf("duplicate public model alias %q", definition.ID)
		}
		registry.byAlias[alias] = len(registry.ordered)
		registry.byUpstream[definition.UpstreamID] = len(registry.ordered)
		registry.ordered = append(registry.ordered, definition)
	}

	return registry, nil
}

// Resolve matches a public model identifier case-insensitively. Upstream
// provider identifiers are deliberately not accepted.
func (r *Registry) Resolve(name string) (*Model, bool) {
	index, ok := r.byAlias[normalizeAlias(name)]
	if !ok {
		return nil, false
	}
	model := r.ordered[index]
	return &model, true
}

// List returns the public models in a stable order.
func (r *Registry) List() []Model {
	list := make([]Model, len(r.ordered))
	copy(list, r.ordered)
	return list
}

func (r *Registry) ChatModels() []Model {
	chat := make([]Model, 0, 3)
	for _, model := range r.ordered {
		if model.Kind == KindChat {
			chat = append(chat, model)
		}
	}
	return chat
}

func (r *Registry) PublicIDs() []string {
	ids := make([]string, 0, len(r.ordered))
	for _, model := range r.ordered {
		ids = append(ids, model.ID)
	}
	return ids
}

// MissingPublic returns the public IDs whose configured upstream identifier is
// absent from the supplied upstream catalog. Comparison is exact because
// provider model IDs are case-sensitive.
func (r *Registry) MissingPublic(upstreamIDs []string) []string {
	available := make(map[string]struct{}, len(upstreamIDs))
	for _, id := range upstreamIDs {
		available[id] = struct{}{}
	}
	missing := make([]string, 0, len(r.ordered))
	for _, model := range r.ordered {
		if _, ok := available[model.UpstreamID]; !ok {
			missing = append(missing, model.ID)
		}
	}
	return missing
}

func normalizeAlias(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
