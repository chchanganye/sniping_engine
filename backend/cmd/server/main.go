package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sniping_engine/internal/config"
	"sniping_engine/internal/engine"
	"sniping_engine/internal/httpapi"
	"sniping_engine/internal/logbus"
	"sniping_engine/internal/provider/standard"
	"sniping_engine/internal/store/sqlite"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	bus := logbus.New(200)
	bus.Log("info", "server starting", map[string]any{"addr": cfg.Server.Addr})

	ctx := context.Background()
	store, err := sqlite.Open(ctx, cfg.Storage.SQLitePath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	prov := standard.New(cfg.Provider, cfg.Proxy, bus)
	eng := engine.New(engine.Options{
		Store:    store,
		Provider: prov,
		Bus:      bus,
		Limits:   cfg.Limits,
		Task:     cfg.Task,
	})

	api := httpapi.New(httpapi.Options{
		Cfg:    cfg,
		Bus:    bus,
		Store:  store,
		Engine: eng,
	})

	server := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		serverErr <- server.ListenAndServe()
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		bus.Log("info", "shutdown signal received", map[string]any{"signal": sig.String()})
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			bus.Log("error", "http server error", map[string]any{"error": err.Error()})
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = eng.StopAll(shutdownCtx)
	_ = server.Shutdown(shutdownCtx)
	bus.Log("info", "server stopped", nil)
}

