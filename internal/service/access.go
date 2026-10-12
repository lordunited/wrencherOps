// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"context"

	"devops-wrapper/internal/access"
)

// appTemplates maps every app's ID to its template.
func (s *AppStore) templates(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, template FROM compose_apps")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var id, tpl string
		if err := rows.Scan(&id, &tpl); err != nil {
			return nil, err
		}
		out[id] = tpl
	}
	return out, rows.Err()
}

func tabOf(s *Service, templates map[string]string) string {
	return access.ServiceTab(s.Type, s.AppID, templates[s.AppID], s.Origin)
}

// ServiceTab is the panel tab service id is in, for the access guard.
func (h *Handler) ServiceTab(ctx context.Context, id string) (string, error) {
	svc, ok := h.services.Get(ctx, id)
	if !ok {
		return "", access.ErrNotFound
	}
	tpl := ""
	if svc.AppID != "" {
		if a, ok := h.apps.Get(ctx, svc.AppID); ok {
			tpl = a.Template
		}
	}
	return access.ServiceTab(svc.Type, svc.AppID, tpl, svc.Origin), nil
}

// AppTab is the panel tab compose app id is in, for the access guard.
func (h *Handler) AppTab(ctx context.Context, id string) (string, error) {
	a, ok := h.apps.Get(ctx, id)
	if !ok {
		return "", access.ErrNotFound
	}
	return appTab(a), nil
}

func appTab(a *App) string { return access.ServiceTab("docker", a.ID, a.Template, "") }

// visibleServices is services, less those in tabs the request's user can't
// see.
func (h *Handler) visibleServices(ctx context.Context, services []*Service) ([]*Service, error) {
	if access.FromContext(ctx) == nil {
		return services, nil
	}
	templates, err := h.apps.templates(ctx)
	if err != nil {
		return nil, err
	}
	return access.Filter(ctx, services, func(s *Service) string { return tabOf(s, templates) }), nil
}
