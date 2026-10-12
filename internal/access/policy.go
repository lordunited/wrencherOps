// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package access

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// Rule is what one API route needs: a level in (any of) some tabs. Resolve,
// when set, works the tabs out from the request instead — the service an
// {id} is, or the kind of thing a body creates. No tabs and a level of
// None means any signed-in user.
type Rule struct {
	Tabs    []string
	Need    Level
	Resolve func(ctx context.Context, r *http.Request) ([]string, error)
}

// Lookups find the tab of the things routes name by ID.
type Lookups struct {
	// ServiceTab is the tab of service id (see ServiceTab).
	ServiceTab func(ctx context.Context, id string) (string, error)
	// AppTab is the tab of compose app id.
	AppTab func(ctx context.Context, id string) (string, error)
}

func view(tabs ...string) Rule  { return Rule{Tabs: tabs, Need: View} }
func write(tabs ...string) Rule { return Rule{Tabs: tabs, Need: Write} }

var signedIn = Rule{}

// maxPeek bounds how much of a body is read to find what it creates.
const maxPeek = 16 << 20

// peekField reads one string field of a JSON body, leaving the body for
// the handler to read as if untouched.
func peekField(r *http.Request, field string) string {
	if r.Body == nil {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxPeek))
	r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(b), r.Body))
	if err != nil {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	var s string
	json.Unmarshal(m[field], &s)
	return s
}

// Policy is the rule for every API route, by its exact ServeMux pattern.
// A route with no rule is refused: TestEveryRouteHasARule (cmd/server)
// fails if one is added without one.
func Policy(l Lookups) map[string]Rule {
	byService := func(need Level) Rule {
		return Rule{Need: need, Resolve: func(ctx context.Context, r *http.Request) ([]string, error) {
			tab, err := l.ServiceTab(ctx, r.PathValue("id"))
			return []string{tab}, err
		}}
	}
	byApp := func(need Level) Rule {
		return Rule{Need: need, Resolve: func(ctx context.Context, r *http.Request) ([]string, error) {
			tab, err := l.AppTab(ctx, r.PathValue("id"))
			return []string{tab}, err
		}}
	}
	// Connecting an existing stack is in the tab of what it is.
	byKind := Rule{Need: Write, Resolve: func(ctx context.Context, r *http.Request) ([]string, error) {
		kind := peekField(r, "kind")
		if kind == "" {
			kind = "prometheus"
		}
		return []string{"deploy." + kind}, nil
	}}
	// A new app is SigNoz's, or a custom compose one.
	byTemplate := Rule{Need: Write, Resolve: func(ctx context.Context, r *http.Request) ([]string, error) {
		if peekField(r, "template") == "signoz" {
			return []string{"deploy.signoz"}, nil
		}
		return []string{"deploy.compose"}, nil
	}}
	deployTabs := DeployTabs()

	p := map[string]Rule{
		"GET /api/auth/me":           signedIn,
		"POST /api/account/password": signedIn,

		// GitLab: read only.
		"GET /api/gitlab/health":                       view("gitlab.health"),
		"GET /api/gitlab/projects":                     view("gitlab.projects"),
		"GET /api/gitlab/projects/{id}":                view("gitlab.projects"),
		"GET /api/gitlab/projects/{id}/pipelines":      view("gitlab.projects"),
		"GET /api/gitlab/projects/{id}/merge-requests": view("gitlab.projects"),
		"GET /api/gitlab/projects/{id}/members":        view("gitlab.projects"),
		"GET /api/gitlab/runners":                      view("gitlab.runners"),
		"GET /api/gitlab/pipelines":                    view("gitlab.pipelines"),
		"GET /api/gitlab/merge-requests":               view("gitlab.merge-requests"),
		"GET /api/gitlab/users":                        view("gitlab.users"),

		// A deploy's progress: anyone who sees a deploy tab.
		"GET /api/deploy/jobs/{id}": view(deployTabs...),

		// Services: the list is filtered to the tabs the user sees.
		"GET /api/services":                   signedIn,
		"POST /api/services/gitlab":           write("deploy.gitlab"),
		"POST /api/services/nexus3":           write("deploy.nexus3"),
		"POST /api/services/prometheus":       write("deploy.prometheus"),
		"POST /api/services/nginx":            write("deploy.nginx"),
		"POST /api/services/haproxy":          write("deploy.haproxy"),
		"POST /api/services/envoy":            write("deploy.envoy"),
		"POST /api/services/signoz":           write("deploy.signoz"),
		"POST /api/services/docker":           write("deploy.other"),
		"POST /api/services/kubernetes":       write("deploy.other"),
		"POST /api/services/connect/discover": byKind,
		"POST /api/services/connect":          byKind,
		"GET /api/services/{id}":              byService(View),
		"PUT /api/services/{id}":              byService(Write),
		"DELETE /api/services/{id}":           byService(Write),
		"POST /api/services/{id}/deploy":      byService(Write),
		"POST /api/services/{id}/start":       byService(Write),
		"POST /api/services/{id}/stop":        byService(Write),
		"POST /api/services/{id}/down":        byService(Write),
		"POST /api/services/{id}/destroy":     byService(Write),
		// Sync only checks health again: reading, not changing.
		"POST /api/services/{id}/sync":        byService(View),
		"GET /api/services/{id}/logs":         byService(View),
		"GET /api/services/{id}/logs/{logId}": byService(View),

		// Each stack's config: inspect and preview read; test runs a
		// container, apply changes the server.
		"POST /api/services/{id}/prometheus/inspect":      byService(View),
		"POST /api/services/{id}/prometheus/preview":      byService(View),
		"POST /api/services/{id}/prometheus/apply":        byService(Write),
		"POST /api/services/{id}/prometheus/move-to-disk": byService(Write),
		"GET /api/services/{id}/prometheus/api/{rest...}": byService(View),
		"POST /api/services/{id}/prometheus/status":       byService(View),
		"GET /api/services/{id}/prometheus/snapshot":      byService(View),
		"POST /api/services/{id}/nginx/inspect":           byService(View),
		"POST /api/services/{id}/nginx/preview":           byService(View),
		"POST /api/services/{id}/nginx/apply":             byService(Write),
		"GET /api/services/{id}/nginx/snapshot":           byService(View),
		"POST /api/services/{id}/haproxy/inspect":         byService(View),
		"POST /api/services/{id}/haproxy/preview":         byService(View),
		"POST /api/services/{id}/haproxy/test":            byService(Write),
		"POST /api/services/{id}/haproxy/apply":           byService(Write),
		"GET /api/services/{id}/haproxy/snapshot":         byService(View),
		"POST /api/services/{id}/envoy/inspect":           byService(View),
		"POST /api/services/{id}/envoy/preview":           byService(View),
		"POST /api/services/{id}/envoy/test":              byService(Write),
		"POST /api/services/{id}/envoy/apply":             byService(Write),
		"GET /api/services/{id}/envoy/snapshot":           byService(View),

		// Compose apps: the list is filtered like services'.
		"GET /api/apps":                  signedIn,
		"POST /api/apps":                 byTemplate,
		"GET /api/apps/{id}":             byApp(View),
		"PUT /api/apps/{id}":             byApp(Write),
		"DELETE /api/apps/{id}":          byApp(Write),
		"POST /api/apps/{id}/servers":    byApp(Write),
		"GET /api/apps/templates/signoz": view("deploy.signoz"),
		"POST /api/apps/detect":          write("deploy.compose"),
		"POST /api/apps/detect/add":      write("deploy.compose"),
		"POST /api/apps/import":          write("deploy.compose"),
		"POST /api/apps/watch":           write("deploy.compose"),
		// .env files hold secrets: only those who can change them read them.
		"POST /api/apps/envfile":      write("deploy.compose", "deploy.signoz"),
		"POST /api/apps/envfile/save": write("deploy.compose", "deploy.signoz"),

		// Servers. A check only reads the server again.
		"GET /api/servers":                  view("servers", "servers.notifications", "ansible.playbooks"), // see servers.handleList
		"POST /api/servers":                 write("servers"),
		"GET /api/servers/alerts":           view("servers", "servers.notifications"),
		"POST /api/servers/alerts/{id}/ack": write("servers", "servers.notifications"),
		"GET /api/servers/rules":            view("servers.notifications"),
		"POST /api/servers/rules":           write("servers.notifications"),
		"PUT /api/servers/rules/{id}":       write("servers.notifications"),
		"DELETE /api/servers/rules/{id}":    write("servers.notifications"),
		"POST /api/servers/check":           view("servers"),
		"GET /api/servers/{id}":             view("servers"),
		"PUT /api/servers/{id}":             write("servers"),
		"DELETE /api/servers/{id}":          write("servers"),
		"POST /api/servers/{id}/check":      view("servers"),

		// Ansible.
		"GET /api/ansible/templates":            view("ansible.playbooks"),
		"GET /api/ansible/modules":              view("ansible.modules", "ansible.playbooks"),
		"GET /api/ansible/modules/{name}":       view("ansible.modules", "ansible.playbooks"),
		"GET /api/ansible/playbooks":            view("ansible.playbooks"),
		"POST /api/ansible/playbooks":           write("ansible.playbooks"),
		"POST /api/ansible/playbooks/upload":    write("ansible.playbooks"),
		"GET /api/ansible/playbooks/{id}":       view("ansible.playbooks"),
		"PUT /api/ansible/playbooks/{id}":       write("ansible.playbooks"),
		"DELETE /api/ansible/playbooks/{id}":    write("ansible.playbooks"),
		"POST /api/ansible/playbooks/{id}/runs": write("ansible.playbooks"),
		"GET /api/ansible/runs":                 view("ansible.runs", "ansible.playbooks"),
		"GET /api/ansible/runs/{id}":            view("ansible.runs", "ansible.playbooks"),
		"POST /api/ansible/runs/{id}/cancel":    write("ansible.playbooks"),

		// Terraform.
		"GET /api/terraform/templates": view("terraform.workspaces"),
		"GET /api/terraform/standard":  view("terraform.workspaces"),
		"POST /api/terraform/fmt":      write("terraform.workspaces"),
		// Checking code against the standard only reads.
		"POST /api/terraform/workspaces/{id}/analyze":  view("terraform.workspaces"),
		"GET /api/terraform/workspaces":                view("terraform.workspaces"),
		"POST /api/terraform/workspaces":               write("terraform.workspaces"),
		"POST /api/terraform/workspaces/upload":        write("terraform.workspaces"),
		"GET /api/terraform/workspaces/{id}":           view("terraform.workspaces"),
		"PUT /api/terraform/workspaces/{id}":           write("terraform.workspaces"),
		"DELETE /api/terraform/workspaces/{id}":        write("terraform.workspaces"),
		"GET /api/terraform/workspaces/{id}/variables": view("terraform.workspaces"),
		"PUT /api/terraform/workspaces/{id}/variables": write("terraform.workspaces"),
		"GET /api/terraform/workspaces/{id}/state":     view("terraform.workspaces"),
		"POST /api/terraform/workspaces/{id}/runs":     write("terraform.workspaces"),
		"GET /api/terraform/runs":                      view("terraform.runs", "terraform.workspaces"),
		"GET /api/terraform/runs/{id}":                 view("terraform.runs", "terraform.workspaces"),
		"POST /api/terraform/runs/{id}/apply":          write("terraform.workspaces"),
		"POST /api/terraform/runs/{id}/cancel":         write("terraform.workspaces"),
	}
	for pattern := range AdminRoutes {
		p[pattern] = write("admin.users")
	}
	return p
}
