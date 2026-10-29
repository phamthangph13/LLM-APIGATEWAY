package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/deepsi/llm-api-gateway/internal/config"
	"github.com/deepsi/llm-api-gateway/internal/models"
	"github.com/deepsi/llm-api-gateway/internal/providers/ninerouter"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	client, err := ninerouter.New(cfg.Upstream.BaseURL, cfg.Upstream.APIKey)
	if err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		os.Exit(1)
	}
	registry, err := models.NewRegistry(cfg.Models.M1, cfg.Models.S1, cfg.Models.V1, cfg.Models.E1)
	if err != nil {
		fmt.Fprintf(os.Stderr, "registry error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	upstreamIDs, err := client.ListModels(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "9Router discovery failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("9Router reachable: %d model(s) discovered\n\n", len(upstreamIDs))
	missing := registry.MissingPublic(upstreamIDs)
	missingSet := make(map[string]struct{}, len(missing))
	for _, id := range missing {
		missingSet[id] = struct{}{}
	}

	for _, model := range registry.List() {
		state := "OK"
		if _, absent := missingSet[model.ID]; absent {
			state = "MISSING"
		}
		fmt.Printf("%-10s -> %-60s %s\n", model.ID, model.UpstreamID, state)
	}

	if len(missing) > 0 {
		fmt.Printf("\n%d configured upstream model(s) are not registered in 9Router.\n", len(missing))
		fmt.Println("Register them in the 9Router dashboard or override the DEEPSI_*_UPSTREAM_MODEL variables.")
		os.Exit(2)
	}
}
