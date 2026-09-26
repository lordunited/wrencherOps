// Package gitlab wraps the GitLab API for the endpoints this tool exposes.
package gitlab

import (
	"fmt"
	"sync"
	"time"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/sync/errgroup"

	"devops-wrapper/internal/cache"
)

const (
	perPage = 100
	// recentPipelinesPerProject caps how many pipelines are fetched per
	// project, so the panel doesn't walk each project's entire history.
	recentPipelinesPerProject = 20
	// recentMergeRequestsPerProject caps how many open merge requests are
	// fetched per project.
	recentMergeRequestsPerProject = 20
	perProjectFetchConcurrency    = 8

	// Caps used on a single project's detail page, where there's no
	// per-project fan-out cost, so a larger window is affordable.
	projectDetailPipelinesLimit     = 50
	projectDetailMergeRequestsLimit = 50
)

type Client struct {
	api   *gitlab.Client
	cache *cache.TTLCache

	// runnerStates and runnerEvents track runner online/offline transitions
	// across fetches, for the health endpoint. They live outside the TTL
	// cache because they're history, not a cacheable value.
	runnerStates sync.Map // runner ID (int64) -> online (bool)
	eventsMu     sync.Mutex
	runnerEvents []HealthEvent
}

func NewClient(baseURL, token string, cacheTTL time.Duration) (*Client, error) {
	api, err := gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, err
	}
	return &Client{api: api, cache: cache.New(cacheTTL)}, nil
}

// paginate walks every page of a GitLab list endpoint and collects the results.
func paginate[T any](fetch func(page int64) ([]T, *gitlab.Response, error)) ([]T, error) {
	all := make([]T, 0)
	var page int64 = 1
	for {
		items, resp, err := fetch(page)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if resp.NextPage == 0 {
			break
		}
		page = resp.NextPage
	}
	return all, nil
}

// fetchPerProject runs fetch for every project concurrently (capped at
// perProjectFetchConcurrency in flight) and collects one result per project.
func fetchPerProject[T any](projects []*gitlab.Project, fetch func(project *gitlab.Project) (T, error)) ([]T, error) {
	result := make([]T, len(projects))
	g := new(errgroup.Group)
	g.SetLimit(perProjectFetchConcurrency)

	for i, project := range projects {
		g.Go(func() error {
			item, err := fetch(project)
			if err != nil {
				return err
			}
			result[i] = item
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) ListProjects() ([]*gitlab.Project, error) {
	return cache.Fetch(c.cache, "projects", func() ([]*gitlab.Project, error) {
		return paginate(func(page int64) ([]*gitlab.Project, *gitlab.Response, error) {
			return c.api.Projects.ListProjects(&gitlab.ListProjectsOptions{
				ListOptions: gitlab.ListOptions{Page: page, PerPage: perPage},
			})
		})
	})
}

func (c *Client) ListRunners() ([]*gitlab.Runner, error) {
	return cache.Fetch(c.cache, "runners", func() ([]*gitlab.Runner, error) {
		runners, err := paginate(func(page int64) ([]*gitlab.Runner, *gitlab.Response, error) {
			return c.api.Runners.ListRunners(&gitlab.ListRunnersOptions{
				ListOptions: gitlab.ListOptions{Page: page, PerPage: perPage},
			})
		})
		if err != nil {
			return nil, err
		}
		c.recordRunnerTransitions(runners)
		return runners, nil
	})
}

func (c *Client) ListUsers() ([]*gitlab.User, error) {
	return cache.Fetch(c.cache, "users", func() ([]*gitlab.User, error) {
		return paginate(func(page int64) ([]*gitlab.User, *gitlab.Response, error) {
			return c.api.Users.ListUsers(&gitlab.ListUsersOptions{
				ListOptions: gitlab.ListOptions{Page: page, PerPage: perPage},
			})
		})
	})
}

// ProjectPipelines groups a project's most recent pipelines, plus its true
// total pipeline count (read from the API's pagination header, not by
// walking every page).
type ProjectPipelines struct {
	ProjectID   int64                  `json:"projectId"`
	ProjectPath string                 `json:"projectPath"`
	TotalCount  int64                  `json:"totalCount"`
	Pipelines   []*gitlab.PipelineInfo `json:"pipelines"`
}

// ListPipelines returns the most recent pipelines for every project the
// token has access to, fetched concurrently (one page per project).
func (c *Client) ListPipelines() ([]ProjectPipelines, error) {
	return cache.Fetch(c.cache, "pipelines", func() ([]ProjectPipelines, error) {
		projects, err := c.ListProjects()
		if err != nil {
			return nil, err
		}

		return fetchPerProject(projects, func(project *gitlab.Project) (ProjectPipelines, error) {
			pipelines, resp, err := c.api.Pipelines.ListProjectPipelines(project.ID, &gitlab.ListProjectPipelinesOptions{
				ListOptions: gitlab.ListOptions{Page: 1, PerPage: recentPipelinesPerProject},
			})
			if err != nil {
				return ProjectPipelines{}, err
			}
			return ProjectPipelines{
				ProjectID:   project.ID,
				ProjectPath: project.PathWithNamespace,
				TotalCount:  resp.TotalItems,
				Pipelines:   pipelines,
			}, nil
		})
	})
}

// ProjectMergeRequests groups a project's most recent open merge requests,
// plus its true open merge request count.
type ProjectMergeRequests struct {
	ProjectID     int64                       `json:"projectId"`
	ProjectPath   string                      `json:"projectPath"`
	TotalCount    int64                       `json:"totalCount"`
	MergeRequests []*gitlab.BasicMergeRequest `json:"mergeRequests"`
}

// ListMergeRequests returns the most recent open merge requests for every
// project the token has access to, fetched concurrently (one page per project).
func (c *Client) ListMergeRequests() ([]ProjectMergeRequests, error) {
	return cache.Fetch(c.cache, "merge_requests", func() ([]ProjectMergeRequests, error) {
		projects, err := c.ListProjects()
		if err != nil {
			return nil, err
		}

		opened := "opened"
		return fetchPerProject(projects, func(project *gitlab.Project) (ProjectMergeRequests, error) {
			mrs, resp, err := c.api.MergeRequests.ListProjectMergeRequests(project.ID, &gitlab.ListProjectMergeRequestsOptions{
				ListOptions: gitlab.ListOptions{Page: 1, PerPage: recentMergeRequestsPerProject},
				State:       &opened,
			})
			if err != nil {
				return ProjectMergeRequests{}, err
			}
			return ProjectMergeRequests{
				ProjectID:     project.ID,
				ProjectPath:   project.PathWithNamespace,
				TotalCount:    resp.TotalItems,
				MergeRequests: mrs,
			}, nil
		})
	})
}

// GetProject returns a single project's details.
func (c *Client) GetProject(id int64) (*gitlab.Project, error) {
	return cache.Fetch(c.cache, fmt.Sprintf("project:%d", id), func() (*gitlab.Project, error) {
		project, _, err := c.api.Projects.GetProject(id, &gitlab.GetProjectOptions{})
		return project, err
	})
}

// ListProjectPipelines returns the most recent pipelines for a single project.
func (c *Client) ListProjectPipelines(id int64) ([]*gitlab.PipelineInfo, error) {
	return cache.Fetch(c.cache, fmt.Sprintf("project:%d:pipelines", id), func() ([]*gitlab.PipelineInfo, error) {
		pipelines, _, err := c.api.Pipelines.ListProjectPipelines(id, &gitlab.ListProjectPipelinesOptions{
			ListOptions: gitlab.ListOptions{Page: 1, PerPage: projectDetailPipelinesLimit},
		})
		return pipelines, err
	})
}

// ListProjectMergeRequests returns the most recent open merge requests for a single project.
func (c *Client) ListProjectMergeRequests(id int64) ([]*gitlab.BasicMergeRequest, error) {
	return cache.Fetch(c.cache, fmt.Sprintf("project:%d:merge_requests", id), func() ([]*gitlab.BasicMergeRequest, error) {
		opened := "opened"
		mrs, _, err := c.api.MergeRequests.ListProjectMergeRequests(id, &gitlab.ListProjectMergeRequestsOptions{
			ListOptions: gitlab.ListOptions{Page: 1, PerPage: projectDetailMergeRequestsLimit},
			State:       &opened,
		})
		return mrs, err
	})
}

// ListProjectMembers returns everyone with access to a project, including
// access inherited from its group.
func (c *Client) ListProjectMembers(id int64) ([]*gitlab.ProjectMember, error) {
	return cache.Fetch(c.cache, fmt.Sprintf("project:%d:members", id), func() ([]*gitlab.ProjectMember, error) {
		return paginate(func(page int64) ([]*gitlab.ProjectMember, *gitlab.Response, error) {
			return c.api.ProjectMembers.ListAllProjectMembers(id, &gitlab.ListProjectMembersOptions{
				ListOptions: gitlab.ListOptions{Page: page, PerPage: perPage},
			})
		})
	})
}
