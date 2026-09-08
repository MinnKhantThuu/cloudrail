package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cloudrail/internal/api"
	"cloudrail/internal/auth"
	"cloudrail/internal/builds"
	"cloudrail/internal/deployment"
	"cloudrail/internal/githubapp"
	"cloudrail/internal/node"
	"cloudrail/internal/secrets"
)

func main() {
	admin, agent := os.Getenv("CLOUDRAIL_ADMIN_TOKEN"), os.Getenv("CLOUDRAIL_AGENT_TOKEN")
	if len(admin) < 32 || len(agent) < 32 || admin == agent {
		slog.Error("Set distinct admin and agent tokens, each at least 32 characters")
		os.Exit(1)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	initCtx, initCancel := context.WithTimeout(ctx, 30*time.Second)
	defer initCancel()
	store, err := deployment.Open(initCtx, os.Getenv("DATABASE_URL"))
	if err != nil {
		slog.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer store.DB.Close()
	store.Cipher, err = secrets.New(os.Getenv("CLOUDRAIL_ENCRYPTION_KEY"))
	if err != nil {
		slog.Error("encryption initialization failed", "error", err)
		os.Exit(1)
	}
	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	web := os.Getenv("WEB_DIR")
	if web == "" {
		web = "apps/web/dist"
	}
	sessions := &auth.Auth{DB: store.DB, Secure: os.Getenv("SECURE_COOKIES") == "true"}
	stateDir := os.Getenv("SERVER_STATE_DIR")
	if stateDir == "" {
		stateDir = ".data/server"
	}
	authority, err := node.Load(stateDir)
	if err != nil {
		slog.Error("CA initialization failed", "error", err)
		os.Exit(1)
	}
	tlsConfig, err := authority.ServerTLS()
	if err != nil {
		slog.Error("TLS initialization failed", "error", err)
		os.Exit(1)
	}
	buildStore := &builds.Store{DB: store.DB, Deployments: store}
	go func() {
		for ctx.Err() == nil {
			if e := buildStore.Flush(ctx); e != nil && ctx.Err() == nil {
				slog.Warn("built image deployment pending", "error", e)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
	a := &api.API{Builds: buildStore, GitHub: githubapp.New(store.DB, store.Cipher), Authority: authority, Store: store, AdminToken: admin, AgentToken: agent, Sessions: sessions}
	server := &http.Server{Addr: addr, Handler: a.Handler(web), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	private := &http.Server{Addr: ":8443", Handler: a.Handler(web), TLSConfig: tlsConfig, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if e := private.ListenAndServeTLS("", ""); e != nil && !errors.Is(e, http.ErrServerClosed) {
			slog.Error("node listener failed", "error", e)
			cancel()
		}
	}()
	go func() {
		<-ctx.Done()
		stopCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = server.Shutdown(stopCtx)
		_ = private.Shutdown(stopCtx)
	}()
	slog.Info("Cloudrail API ready", "address", addr)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
