// Package main ties together all dependencies and starts the ct-server.
// This is the only place where concrete implementations are wired to domain interfaces.
package main

import (
	"fmt"
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting ct-server")

	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// TODO: Load config (JSON + JSON Schema validation).

	// TODO: Open SQLite database.
	// db := sqlite.Open(cfg.DBPath)
	// defer db.Close()

	// TODO: Wire domain service implementations.
	// var userService crosstalk.UserService = &sqlite.UserService{DB: db}
	// var templateService crosstalk.SessionTemplateService = &sqlite.SessionTemplateService{DB: db}
	// var sessionService crosstalk.SessionService = &sqlite.SessionService{DB: db}

	// TODO: Create HTTP handler, inject services.
	// var handler http.Handler
	// handler.UserService = userService
	// handler.SessionTemplateService = templateService

	// TODO: Start HTTP server (serves REST API + embedded web UI).

	fmt.Println("ct-server: not yet implemented")
	return nil
}
