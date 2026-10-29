package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/deepsi/llm-api-gateway/internal/config"
	"github.com/deepsi/llm-api-gateway/internal/server"
)

func main() {
	showVersion := flag.Bool("version", false, "print the gateway version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(server.Version)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	loggerOptions := &slog.HandlerOptions{Level: cfg.App.LogLevel}
	var handler slog.Handler
	if cfg.App.IsProduction {
		handler = slog.NewJSONHandler(os.Stdout, loggerOptions)
	} else {
		handler = slog.NewTextHandler(os.Stdout, loggerOptions)
	}
	log := slog.New(handler)
	slog.SetDefault(log)

	for _, warning := range cfg.Warnings {
		log.Warn(warning)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := server.New(&cfg, log)
	if err != nil {
		log.Error("initialize gateway", "error", err)
		os.Exit(1)
	}
	defer application.Close()

	if err := application.Run(ctx); err != nil {
		log.Error("run gateway", "error", err)
		os.Exit(1)
	}
}
