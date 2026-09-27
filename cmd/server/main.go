package main

import (
	"context"
	"log"
	"net/http"

	"devops-wrapper/internal/auth"
	"devops-wrapper/internal/config"
	"devops-wrapper/internal/db"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/gitlab"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/service"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	glClient, err := gitlab.NewClient(cfg.GitLabBaseURL, cfg.GitLabToken, cfg.CacheTTL)
	if err != nil {
		log.Fatalf("gitlab client: %v", err)
	}

	users, err := auth.ParseUsers(cfg.AuthUsersRaw)
	if err != nil {
		log.Fatalf("auth: %v", err)
	}
	sessions := auth.NewSessionStore(pool, cfg.SessionTTL)
	authHandler := auth.NewHandler(users, sessions, cfg.CookieSecure)

	secrets, err := secretbox.New(cfg.CredentialKey)
	if err != nil {
		log.Fatalf("secretbox: %v", err)
	}

	serviceStore := service.NewStore(pool)
	logStore := service.NewLogStore(pool)
	if n, err := logStore.MarkInterrupted(ctx); err != nil {
		log.Fatalf("service logs: %v", err)
	} else if n > 0 {
		log.Printf("service logs: marked %d interrupted action(s) as failed", n)
	}
	jobStore := deploy.NewStore()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	authHandler.RegisterRoutes(mux)

	gitlabMux := http.NewServeMux()
	glClient.RegisterRoutes(gitlabMux)
	mux.Handle("/api/gitlab/", authHandler.RequireAuth(gitlabMux))

	deployHandler := deploy.NewHandler(jobStore)
	deployMux := http.NewServeMux()
	deployHandler.RegisterRoutes(deployMux)
	mux.Handle("/api/deploy/", authHandler.RequireAuth(deployMux))

	serviceHandler := service.NewHandler(serviceStore, logStore, service.NewSnapshotStore(pool), jobStore, secrets)
	// Watches connected stacks until their Start/Stop/Down unlock.
	serviceHandler.StartMonitor(ctx)
	serviceMux := http.NewServeMux()
	serviceHandler.RegisterRoutes(serviceMux)
	mux.Handle("/api/services", authHandler.RequireAuth(serviceMux))
	mux.Handle("/api/services/", authHandler.RequireAuth(serviceMux))

	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
