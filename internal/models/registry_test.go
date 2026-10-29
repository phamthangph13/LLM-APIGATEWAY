package models

import (
	"strings"
	"testing"
)

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(
		"nvidia/m1",
		"stepfun/s1",
		"nvidia/v1",
		"nvidia/e1",
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func TestRegistryMappings(t *testing.T) {
	registry := testRegistry(t)

	expected := map[string]string{
		"DeepSI-M1": "nvidia/m1",
		"DeepSI-S1": "stepfun/s1",
		"DeepSI-V1": "nvidia/v1",
		"DeepSI-E1": "nvidia/e1",
	}
	for public, upstream := range expected {
		model, ok := registry.Resolve(public)
		if !ok {
			t.Fatalf("Resolve(%q) failed", public)
		}
		if model.UpstreamID != upstream {
			t.Fatalf("%s mapped to %s, want %s", public, model.UpstreamID, upstream)
		}
	}
}

func TestRegistryResolveIsCaseInsensitiveAndTrimmed(t *testing.T) {
	registry := testRegistry(t)

	for _, alias := range []string{"deepsi-m1", "DEEPSI-M1", "  DeepSI-M1  ", "DeEpSi-M1"} {
		model, ok := registry.Resolve(alias)
		if !ok {
			t.Fatalf("Resolve(%q) failed", alias)
		}
		if model.ID != "DeepSI-M1" {
			t.Fatalf("Resolve(%q) = %s", alias, model.ID)
		}
	}
}

func TestRegistryDoesNotResolveUpstreamIDs(t *testing.T) {
	registry := testRegistry(t)

	for _, upstream := range []string{"nvidia/m1", "stepfun/s1", "nvidia/v1", "nvidia/e1"} {
		if _, ok := registry.Resolve(upstream); ok {
			t.Fatalf("upstream ID %q must not resolve as a public alias", upstream)
		}
	}
}

func TestRegistryListIsStableAndPublic(t *testing.T) {
	registry := testRegistry(t)
	list := registry.List()

	if len(list) != 4 {
		t.Fatalf("List() returned %d models, want 4", len(list))
	}
	ids := registry.PublicIDs()
	want := []string{"DeepSI-M1", "DeepSI-S1", "DeepSI-V1", "DeepSI-E1"}
	for index, id := range want {
		if ids[index] != id {
			t.Fatalf("PublicIDs()[%d] = %s, want %s", index, ids[index], id)
		}
		if list[index].UpstreamID == "" {
			t.Fatalf("%s has no upstream ID", id)
		}
	}
}

func TestRegistryChatModelsHaveIdentityPrompts(t *testing.T) {
	registry := testRegistry(t)

	chat := registry.ChatModels()
	if len(chat) != 3 {
		t.Fatalf("ChatModels() returned %d models, want 3", len(chat))
	}
	for _, model := range chat {
		if model.SystemPrompt == "" {
			t.Fatalf("%s has no system prompt", model.ID)
		}
		if !strings.Contains(model.SystemPrompt, "You are "+model.ID) {
			t.Fatalf("%s identity prompt does not declare its public name", model.ID)
		}
		if !strings.Contains(model.SystemPrompt, model.ID) {
			t.Fatalf("%s identity prompt does not contain its public name", model.ID)
		}
	}

	embedding, ok := registry.Resolve("DeepSI-E1")
	if !ok {
		t.Fatal("DeepSI-E1 is missing")
	}
	if embedding.Kind != KindEmbeddings {
		t.Fatalf("DeepSI-E1 kind = %s", embedding.Kind)
	}
	if embedding.SystemPrompt != "" {
		t.Fatal("embedding model must not have a chat system prompt")
	}
}

func TestRegistryMissingPublic(t *testing.T) {
	registry := testRegistry(t)

	missing := registry.MissingPublic([]string{"nvidia/m1", "unknown/model"})
	want := map[string]bool{"DeepSI-M1": false, "DeepSI-S1": true, "DeepSI-V1": true, "DeepSI-E1": true}
	for _, id := range missing {
		if _, ok := want[id]; !ok {
			t.Fatalf("unexpected missing model %s", id)
		}
		want[id] = false
	}
	for id, stillMissing := range want {
		if stillMissing {
			t.Fatalf("%s should not be missing", id)
		}
	}
}
