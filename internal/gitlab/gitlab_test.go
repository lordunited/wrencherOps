// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package gitlab

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

func TestFailingProjects(t *testing.T) {
	p := func(status string) *gitlab.PipelineInfo {
		return &gitlab.PipelineInfo{Status: status, WebURL: "https://g/" + status}
	}
	got := failingProjects([]ProjectPipelines{
		{ProjectPath: "a/one-failure", Pipelines: []*gitlab.PipelineInfo{p("failed"), p("success")}},
		{ProjectPath: "a/two", Pipelines: []*gitlab.PipelineInfo{p("failed"), p("failed"), p("success"), p("failed")}},
		{ProjectPath: "a/three", Pipelines: []*gitlab.PipelineInfo{p("failed"), p("failed"), p("failed")}},
		{ProjectPath: "a/recovered", Pipelines: []*gitlab.PipelineInfo{p("success"), p("failed"), p("failed")}},
		{ProjectPath: "a/none"},
	})
	if len(got) != 2 || got[0].ProjectPath != "a/three" || got[0].ConsecutiveFailures != 3 || got[1].ProjectPath != "a/two" || got[1].LastPipelineWebURL != "https://g/failed" {
		t.Errorf("failingProjects = %+v", got)
	}
}

func TestStaleMergeRequests(t *testing.T) {
	ago := func(days int) *time.Time { t := time.Now().Add(-time.Duration(days) * 24 * time.Hour); return &t }
	got := staleMergeRequests([]ProjectMergeRequests{{ProjectPath: "a/b", MergeRequests: []*gitlab.BasicMergeRequest{
		{Title: "fresh", CreatedAt: ago(3)},
		{Title: "old", CreatedAt: ago(20), Author: &gitlab.BasicUser{Username: "sara"}},
		{Title: "older", CreatedAt: ago(40)},
		{Title: "no date"},
	}}})
	if len(got) != 2 || got[0].Title != "older" || got[1].Title != "old" || got[1].Author != "sara" || got[1].AgeDays < 19 {
		t.Errorf("staleMergeRequests = %+v", got)
	}
}

func TestRunnerTransitions(t *testing.T) {
	c := &Client{}
	c.recordRunnerTransitions([]*gitlab.Runner{{ID: 1, Online: true, Description: "build-1"}, {ID: 2, Online: false}})
	if len(c.runnerEvents) != 0 {
		t.Fatal("the first sighting isn't a transition")
	}
	c.recordRunnerTransitions([]*gitlab.Runner{{ID: 1, Online: false, Description: "build-1"}, {ID: 2, Online: true}})
	if len(c.runnerEvents) != 2 || c.runnerEvents[0].Kind != "runner_offline" || !strings.Contains(c.runnerEvents[0].Message, "build-1 went offline") ||
		c.runnerEvents[1].Message != "Runner #2 came back online" {
		t.Errorf("events = %+v", c.runnerEvents)
	}
	for i := 0; i < maxHealthEvents+10; i++ {
		c.appendHealthEvent(HealthEvent{Kind: "x"})
	}
	if len(c.runnerEvents) != maxHealthEvents {
		t.Errorf("%d events kept, want %d", len(c.runnerEvents), maxHealthEvents)
	}
}

// fakeGitLab serves the few API endpoints the client uses: two pages of
// projects, pipelines and merge requests per project.
func fakeGitLab(t *testing.T, requests *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, v any, next string, total int) {
		w.Header().Set("Content-Type", "application/json")
		if next != "" {
			w.Header().Set("X-Next-Page", next)
		}
		w.Header().Set("X-Total", fmt.Sprint(total))
		json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("GET /api/v4/projects", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			http.Error(w, `{"message":"401 Unauthorized"}`, http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("page") == "2" {
			write(w, []map[string]any{{"id": 2, "path_with_namespace": "g/two"}}, "", 2)
			return
		}
		write(w, []map[string]any{{"id": 1, "path_with_namespace": "g/one"}}, "2", 2)
	})
	mux.HandleFunc("GET /api/v4/projects/{id}/pipelines", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		status := "success"
		if r.PathValue("id") == "2" {
			status = "failed"
		}
		write(w, []map[string]any{{"id": 10, "status": status, "web_url": "https://g/p/10"}, {"id": 9, "status": status}}, "", 57)
	})
	mux.HandleFunc("GET /api/v4/projects/{id}/merge_requests", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Query().Get("state") != "opened" {
			t.Errorf("merge requests listed without state=opened: %s", r.URL.RawQuery)
		}
		write(w, []map[string]any{{"iid": 1, "title": "Old one", "created_at": time.Now().Add(-30 * 24 * time.Hour).Format(time.RFC3339)}}, "", 1)
	})
	mux.HandleFunc("GET /api/v4/runners", func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		write(w, []map[string]any{{"id": 5, "description": "r", "online": true}}, "", 1)
	})
	return httptest.NewServer(mux)
}

func TestClientAgainstFakeGitLab(t *testing.T) {
	var requests atomic.Int32
	srv := fakeGitLab(t, &requests)
	defer srv.Close()
	c, err := NewClient(srv.URL, "test-token", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	projects, err := c.ListProjects()
	if err != nil || len(projects) != 2 || projects[1].PathWithNamespace != "g/two" {
		t.Fatalf("ListProjects (two pages) = %v, %v", projects, err)
	}
	n := requests.Load()
	if _, err := c.ListProjects(); err != nil || requests.Load() != n {
		t.Error("a second ListProjects within the TTL hit the API again")
	}
	pipelines, err := c.ListPipelines()
	if err != nil || len(pipelines) != 2 || pipelines[0].TotalCount != 57 {
		t.Fatalf("ListPipelines = %+v, %v", pipelines, err)
	}
	health, err := c.GetHealth()
	if err != nil {
		t.Fatal(err)
	}
	if len(health.FailingProjects) != 1 || health.FailingProjects[0].ProjectPath != "g/two" || len(health.StaleMergeRequests) != 2 {
		t.Errorf("health = %+v", health)
	}

	// The HTTP handlers serve the same, as JSON.
	mux := http.NewServeMux()
	c.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/gitlab/projects", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "g/two") {
		t.Errorf("GET /api/gitlab/projects: %d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/gitlab/projects/not-a-number", nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a bad project ID: %d", rec.Code)
	}
}

func TestClientReportsAPIErrors(t *testing.T) {
	var requests atomic.Int32
	srv := fakeGitLab(t, &requests)
	defer srv.Close()
	c, _ := NewClient(srv.URL, "wrong-token", time.Minute)
	if _, err := c.ListProjects(); err == nil {
		t.Fatal("a refused token gave no error")
	}
	mux := http.NewServeMux()
	c.RegisterRoutes(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/api/gitlab/projects", nil))
	if rec.Code < 400 {
		t.Errorf("an API error answered %d", rec.Code)
	}
}

// A failure says what happened: a proxy's HTML page, a refused token.
func TestErrorsAreExplained(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v4/runners", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, "<html><body><h1>403 Forbidden</h1>\nRequest forbidden by administrative rules.\n</body></html>\n")
	})
	mux.HandleFunc("GET /api/v4/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprint(w, `{"message":"401 Unauthorized"}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c, _ := NewClient(srv.URL, "t", time.Minute)
	routes := http.NewServeMux()
	c.RegisterRoutes(routes)
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		routes.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Body.String()
	}
	code, body := get("/api/gitlab/runners")
	if code != http.StatusBadGateway || !strings.Contains(body, "a proxy in front of it answered 403") ||
		!strings.Contains(body, "Request forbidden by administrative rules") || strings.Contains(body, "<html>") || !strings.Contains(body, "VPN") {
		t.Errorf("proxy 403: %d %s", code, body)
	}
	if code, body := get("/api/gitlab/users"); code != http.StatusBadGateway || !strings.Contains(body, "refused the panel's token") {
		t.Errorf("token refused: %d %s", code, body)
	}
	if got := explain(fmt.Errorf("plain")); got != "plain" {
		t.Errorf("an unknown error: %q", got)
	}
}
