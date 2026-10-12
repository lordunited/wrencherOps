// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package gitlab

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// RegisterRoutes wires the GitLab endpoints onto mux under /api/gitlab.
func (c *Client) RegisterRoutes(mux interface {
	HandleFunc(string, func(http.ResponseWriter, *http.Request))
}) {
	mux.HandleFunc("GET /api/gitlab/projects", c.handleListProjects)
	mux.HandleFunc("GET /api/gitlab/runners", c.handleListRunners)
	mux.HandleFunc("GET /api/gitlab/users", c.handleListUsers)
	mux.HandleFunc("GET /api/gitlab/pipelines", c.handleListPipelines)
	mux.HandleFunc("GET /api/gitlab/merge-requests", c.handleListMergeRequests)
	mux.HandleFunc("GET /api/gitlab/health", c.handleGetHealth)

	mux.HandleFunc("GET /api/gitlab/projects/{id}", c.handleGetProject)
	mux.HandleFunc("GET /api/gitlab/projects/{id}/pipelines", c.handleListProjectPipelines)
	mux.HandleFunc("GET /api/gitlab/projects/{id}/merge-requests", c.handleListProjectMergeRequests)
	mux.HandleFunc("GET /api/gitlab/projects/{id}/members", c.handleListProjectMembers)
}

func (c *Client) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := c.ListProjects()
	writeJSON(w, projects, err)
}

func (c *Client) handleListRunners(w http.ResponseWriter, r *http.Request) {
	runners, err := c.ListRunners()
	writeJSON(w, runners, err)
}

func (c *Client) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := c.ListUsers()
	writeJSON(w, users, err)
}

func (c *Client) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	pipelines, err := c.ListPipelines()
	writeJSON(w, pipelines, err)
}

func (c *Client) handleListMergeRequests(w http.ResponseWriter, r *http.Request) {
	mergeRequests, err := c.ListMergeRequests()
	writeJSON(w, mergeRequests, err)
}

func (c *Client) handleGetHealth(w http.ResponseWriter, r *http.Request) {
	health, err := c.GetHealth()
	writeJSON(w, health, err)
}

func (c *Client) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	project, err := c.GetProject(id)
	writeJSON(w, project, err)
}

func (c *Client) handleListProjectPipelines(w http.ResponseWriter, r *http.Request) {
	id, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	pipelines, err := c.ListProjectPipelines(id)
	writeJSON(w, pipelines, err)
}

func (c *Client) handleListProjectMergeRequests(w http.ResponseWriter, r *http.Request) {
	id, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	mergeRequests, err := c.ListProjectMergeRequests(id)
	writeJSON(w, mergeRequests, err)
}

func (c *Client) handleListProjectMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := projectIDFromRequest(w, r)
	if !ok {
		return
	}
	members, err := c.ListProjectMembers(id)
	writeJSON(w, members, err)
}

func projectIDFromRequest(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid project id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, data any, err error) {
	if err != nil {
		log.Printf("gitlab: %v", err)
		http.Error(w, explain(err), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if encErr := json.NewEncoder(w).Encode(data); encErr != nil {
		log.Printf("gitlab: encode response: %v", encErr)
	}
}
