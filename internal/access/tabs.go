// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package access is the panel's user management: users with a role
// (admin, writer, viewer), groups, and per-tab access (none, view, write),
// enforced on every API call — not only by hiding pages.
package access

import "strings"

// Level is how far a user can go in a tab.
type Level int

const (
	None  Level = iota // the tab is hidden and its API refused
	View               // read only
	Write              // change things: deploy, run, edit, delete
)

func (l Level) String() string {
	switch l {
	case View:
		return "view"
	case Write:
		return "write"
	}
	return "none"
}

// ParseLevel reads "none", "view" or "write".
func ParseLevel(s string) (Level, bool) {
	switch s {
	case "none", "":
		return None, true
	case "view":
		return View, true
	case "write":
		return Write, true
	}
	return None, false
}

// Roles cap what a user can be granted: an admin can do everything
// (including managing users), a writer gets exactly the per-tab access
// granted, a viewer at most view.
const (
	RoleAdmin  = "admin"
	RoleWriter = "writer"
	RoleViewer = "viewer"
)

func ValidRole(r string) bool { return r == RoleAdmin || r == RoleWriter || r == RoleViewer }

// Tab is one page of the panel's sidebar that access is granted on.
type Tab struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Category string `json:"category"`
	Href     string `json:"href"`
	// ViewOnly tabs have nothing to change (GitLab's pages, histories).
	ViewOnly bool `json:"viewOnly,omitempty"`
	// AdminOnly tabs can't be granted: only admins see them.
	AdminOnly bool `json:"adminOnly,omitempty"`
	// Everyone tabs are viewable by every signed-in user.
	Everyone bool `json:"everyone,omitempty"`
}

// Tabs is every tab, in sidebar order. web/lib/nav.ts names the same keys;
// a test keeps the two in step.
var Tabs = []Tab{
	{Key: "overview", Label: "Overview", Category: "General", Href: "/", ViewOnly: true, Everyone: true},

	{Key: "gitlab.health", Label: "Health", Category: "GitLab", Href: "/health", ViewOnly: true},
	{Key: "gitlab.projects", Label: "Projects", Category: "GitLab", Href: "/projects", ViewOnly: true},
	{Key: "gitlab.runners", Label: "Runners", Category: "GitLab", Href: "/runners", ViewOnly: true},
	{Key: "gitlab.pipelines", Label: "Pipelines", Category: "GitLab", Href: "/pipelines", ViewOnly: true},
	{Key: "gitlab.merge-requests", Label: "Merge requests", Category: "GitLab", Href: "/merge-requests", ViewOnly: true},
	{Key: "gitlab.users", Label: "Users", Category: "GitLab", Href: "/users", ViewOnly: true},

	{Key: "servers", Label: "Servers", Category: "Servers", Href: "/servers"},
	{Key: "servers.notifications", Label: "Notifications", Category: "Servers", Href: "/servers/notifications"},

	{Key: "ansible.playbooks", Label: "Playbooks", Category: "Ansible", Href: "/ansible"},
	{Key: "ansible.runs", Label: "Run history", Category: "Ansible", Href: "/ansible/runs", ViewOnly: true},
	{Key: "ansible.modules", Label: "Modules", Category: "Ansible", Href: "/ansible/modules", ViewOnly: true},

	{Key: "terraform.workspaces", Label: "Workspaces", Category: "Terraform", Href: "/terraform"},
	{Key: "terraform.runs", Label: "Run history", Category: "Terraform", Href: "/terraform/runs", ViewOnly: true},

	{Key: "deploy.gitlab", Label: "GitLab", Category: "Deploy", Href: "/deploy/gitlab"},
	{Key: "deploy.nexus3", Label: "Nexus3", Category: "Deploy", Href: "/deploy/nexus3"},
	{Key: "deploy.prometheus", Label: "Prometheus stack", Category: "Deploy", Href: "/deploy/prometheus"},
	{Key: "deploy.nginx", Label: "Nginx", Category: "Deploy", Href: "/deploy/nginx"},
	{Key: "deploy.haproxy", Label: "HAProxy", Category: "Deploy", Href: "/deploy/haproxy"},
	{Key: "deploy.envoy", Label: "Envoy", Category: "Deploy", Href: "/deploy/envoy"},
	{Key: "deploy.signoz", Label: "SigNoz", Category: "Deploy", Href: "/deploy/signoz"},
	{Key: "deploy.compose", Label: "Custom compose", Category: "Deploy", Href: "/deploy/compose"},
	{Key: "deploy.other", Label: "Docker & Kubernetes (raw)", Category: "Deploy", Href: "/deploy/docker"},

	{Key: "admin.users", Label: "Users & access", Category: "Admin", Href: "/admin", AdminOnly: true},
}

// TabByKey finds a tab.
func TabByKey(key string) (Tab, bool) {
	for _, t := range Tabs {
		if t.Key == key {
			return t, true
		}
	}
	return Tab{}, false
}

// TabForHref is the tab a page belongs to: the longest tab href it's at
// or under.
func TabForHref(path string) (Tab, bool) {
	best, found := Tab{}, false
	for _, t := range Tabs {
		if path == t.Href || (t.Href != "/" && strings.HasPrefix(path, t.Href+"/")) {
			if !found || len(t.Href) > len(best.Href) {
				best, found = t, true
			}
		}
	}
	if !found && (path == "/deploy/kubernetes" || strings.HasPrefix(path, "/deploy/kubernetes/")) {
		return TabByKey("deploy.other")
	}
	return best, found
}

// ServiceTab is the tab a deployed service belongs to.
func ServiceTab(svcType, appID, appTemplate, origin string) string {
	switch {
	case appID != "" && appTemplate == "signoz":
		return "deploy.signoz"
	case appID != "":
		return "deploy.compose"
	case svcType == "docker" && origin == "connected":
		return "deploy.compose" // a watched compose project
	case svcType == "docker" || svcType == "kubernetes":
		return "deploy.other"
	}
	return "deploy." + svcType
}

// DeployTabs are every deploy tab.
func DeployTabs() []string {
	var out []string
	for _, t := range Tabs {
		if t.Category == "Deploy" {
			out = append(out, t.Key)
		}
	}
	return out
}
