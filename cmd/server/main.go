// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package main

import (
	"context"
	"log"
	"net/http"

	"devops-wrapper/internal/access"
	"devops-wrapper/internal/ansible"
	"devops-wrapper/internal/auth"
	"devops-wrapper/internal/config"
	"devops-wrapper/internal/db"
	"devops-wrapper/internal/deploy"
	"devops-wrapper/internal/gitlab"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/servers"
	"devops-wrapper/internal/service"
	"devops-wrapper/internal/terraform"
)

func main() {
	protectSecrets()

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
	// Who may do what: AUTH_USERS are built-in admins; the panel's own
	// users, groups and per-tab access are managed in Admin.
	accessStore := access.NewStore(pool)
	resolver := access.NewResolver(users, accessStore)
	authHandler := auth.NewHandler(resolver, sessions, cfg.CookieSecure)

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

	serviceHandler := service.NewHandler(serviceStore, service.NewAppStore(pool), logStore, service.NewSnapshotStore(pool), jobStore, secrets)
	// Watches connected stacks until their Start/Stop/Down unlock.
	serviceHandler.StartMonitor(ctx)

	// Watches the servers themselves: logins, disks, kernel errors,
	// connections, load — and raises alerts from the rules.
	serverStore := servers.NewStore(pool)
	serverHandler := servers.NewHandler(serverStore, secrets)
	if err := serverHandler.Start(ctx); err != nil {
		log.Fatalf("servers: %v", err)
	}

	// Runs Ansible playbooks on the servers above, with their saved logins.
	ansibleHandler := ansible.NewHandler(ansible.NewStore(pool), serverStore, secrets)
	ansibleHandler.Start(ctx)

	// Runs Terraform workspaces with OpenTofu: plan, then apply exactly
	// the reviewed plan; the state is kept here, encrypted.
	tfHandler := terraform.NewHandler(terraform.NewStore(pool), secrets)
	tfHandler.Start(ctx)

	api := apiHandlers{
		gitlab:    glClient,
		deploy:    deploy.NewHandler(jobStore),
		service:   serviceHandler,
		servers:   serverHandler,
		ansible:   ansibleHandler,
		terraform: tfHandler,
		access:    access.NewHandler(accessStore, resolver),
	}
	apiMux := http.NewServeMux()
	api.register(apiMux)
	// Every API route is behind the guard: signed in, and with the access
	// its rule needs in its tab. Login and logout, above, are more
	// specific patterns, so they stay open.
	guard := access.NewGuard(sessions, resolver, access.Policy(access.Lookups{
		ServiceTab: serviceHandler.ServiceTab,
		AppTab:     serviceHandler.AppTab,
	}))
	mux.Handle("/api/", guard.Wrap(apiMux))

	addr := ":" + cfg.Port
	log.Printf("DevOps Panel by Mohammadreza Masoumi (DevOps Engineer) <s.masoumimohammadreza@gmail.com> — listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

// apiHandlers are everything served under /api/ behind the access guard.
type apiHandlers struct {
	gitlab    *gitlab.Client
	deploy    *deploy.Handler
	service   *service.Handler
	servers   *servers.Handler
	ansible   *ansible.Handler
	terraform *terraform.Handler
	access    *access.Handler
}

// routeMux is what the handlers register on; *http.ServeMux is one.
type routeMux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}

func (a apiHandlers) register(mux routeMux) {
	a.gitlab.RegisterRoutes(mux)
	a.deploy.RegisterRoutes(mux)
	a.service.RegisterRoutes(mux)
	a.servers.RegisterRoutes(mux)
	a.ansible.RegisterRoutes(mux)
	a.terraform.RegisterRoutes(mux)
	a.access.RegisterRoutes(mux)
}
