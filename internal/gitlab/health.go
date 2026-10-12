// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package gitlab

import (
	"fmt"
	"sort"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

const (
	// consecutiveFailureThreshold is how many failed pipelines in a row
	// (most recent first) before a project is flagged as failing.
	consecutiveFailureThreshold = 2
	// staleMergeRequestDays is how old an open merge request must be
	// before it's flagged as stale.
	staleMergeRequestDays = 14
	// maxHealthEvents caps how many runner transition events are kept.
	maxHealthEvents = 50
)

type HealthEvent struct {
	Kind      string    `json:"kind"` // "runner_offline" | "runner_online"
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

type FailingProject struct {
	ProjectPath         string `json:"projectPath"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
	LastPipelineWebURL  string `json:"lastPipelineWebUrl"`
}

type StaleMergeRequest struct {
	ProjectPath string `json:"projectPath"`
	Title       string `json:"title"`
	WebURL      string `json:"webUrl"`
	AgeDays     int    `json:"ageDays"`
	Author      string `json:"author"`
}

type Health struct {
	RunnerEvents       []HealthEvent       `json:"runnerEvents"`
	FailingProjects    []FailingProject    `json:"failingProjects"`
	StaleMergeRequests []StaleMergeRequest `json:"staleMergeRequests"`
}

// recordRunnerTransitions compares each runner's online status to what was
// last seen and appends an event for any online<->offline flip. Only called
// from a real fetch (cache miss), never on a cache hit.
func (c *Client) recordRunnerTransitions(runners []*gitlab.Runner) {
	for _, r := range runners {
		prevAny, existed := c.runnerStates.Swap(r.ID, r.Online)
		if !existed {
			continue
		}
		prev, _ := prevAny.(bool)
		if prev == r.Online {
			continue
		}

		desc := r.Description
		if desc == "" {
			desc = fmt.Sprintf("Runner #%d", r.ID)
		}
		kind, verb := "runner_online", "came back online"
		if !r.Online {
			kind, verb = "runner_offline", "went offline"
		}
		c.appendHealthEvent(HealthEvent{
			Kind:      kind,
			Message:   fmt.Sprintf("%s %s", desc, verb),
			Timestamp: time.Now(),
		})
	}
}

func (c *Client) appendHealthEvent(e HealthEvent) {
	c.eventsMu.Lock()
	defer c.eventsMu.Unlock()
	c.runnerEvents = append(c.runnerEvents, e)
	if len(c.runnerEvents) > maxHealthEvents {
		c.runnerEvents = c.runnerEvents[len(c.runnerEvents)-maxHealthEvents:]
	}
}

// GetHealth aggregates signals worth a human's attention: runners that have
// flipped online/offline, projects whose recent pipelines keep failing, and
// open merge requests that have sat around too long.
func (c *Client) GetHealth() (Health, error) {
	// Fetching runners (even from cache) is what drives transition
	// detection on the next real poll.
	if _, err := c.ListRunners(); err != nil {
		return Health{}, err
	}

	pipelineGroups, err := c.ListPipelines()
	if err != nil {
		return Health{}, err
	}
	mergeRequestGroups, err := c.ListMergeRequests()
	if err != nil {
		return Health{}, err
	}

	c.eventsMu.Lock()
	events := make([]HealthEvent, len(c.runnerEvents))
	copy(events, c.runnerEvents)
	c.eventsMu.Unlock()
	sort.Slice(events, func(i, j int) bool { return events[i].Timestamp.After(events[j].Timestamp) })

	return Health{
		RunnerEvents:       events,
		FailingProjects:    failingProjects(pipelineGroups),
		StaleMergeRequests: staleMergeRequests(mergeRequestGroups),
	}, nil
}

func failingProjects(groups []ProjectPipelines) []FailingProject {
	result := make([]FailingProject, 0)
	for _, g := range groups {
		streak := 0
		for _, p := range g.Pipelines {
			if p.Status != "failed" {
				break
			}
			streak++
		}
		if streak < consecutiveFailureThreshold {
			continue
		}
		result = append(result, FailingProject{
			ProjectPath:         g.ProjectPath,
			ConsecutiveFailures: streak,
			LastPipelineWebURL:  g.Pipelines[0].WebURL,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ConsecutiveFailures > result[j].ConsecutiveFailures
	})
	return result
}

func staleMergeRequests(groups []ProjectMergeRequests) []StaleMergeRequest {
	result := make([]StaleMergeRequest, 0)
	now := time.Now()
	for _, g := range groups {
		for _, mr := range g.MergeRequests {
			if mr.CreatedAt == nil {
				continue
			}
			ageDays := int(now.Sub(*mr.CreatedAt).Hours() / 24)
			if ageDays < staleMergeRequestDays {
				continue
			}
			author := ""
			if mr.Author != nil {
				author = mr.Author.Username
			}
			result = append(result, StaleMergeRequest{
				ProjectPath: g.ProjectPath,
				Title:       mr.Title,
				WebURL:      mr.WebURL,
				AgeDays:     ageDays,
				Author:      author,
			})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AgeDays > result[j].AgeDays })
	return result
}
