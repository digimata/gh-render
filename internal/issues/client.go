package issues

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

const issuesPageSize = 100

// RESTDoer is the narrow GitHub REST surface the client consumes.
// *api.RESTClient from go-gh satisfies it.
type RESTDoer interface {
	DoWithContext(
		ctx context.Context,
		method string,
		path string,
		body io.Reader,
		response any,
	) error
}

// Client fetches and normalizes GitHub issues over REST.
type Client struct {
	api RESTDoer
}

// NewClient returns a Client backed by the given REST implementation.
func NewClient(api RESTDoer) *Client {
	return &Client{api: api}
}

// apiIssue is the private REST wire shape of an issue record.
type apiIssue struct {
	Number    int             `json:"number"`
	Title     string          `json:"title"`
	Body      *string         `json:"body"`
	State     string          `json:"state"`
	HTMLURL   string          `json:"html_url"`
	User      apiUser         `json:"user"`
	Labels    []apiLabel      `json:"labels"`
	Assignees []apiUser       `json:"assignees"`
	Milestone *apiMilestone   `json:"milestone"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Pull      json.RawMessage `json:"pull_request"`
}

type apiUser struct {
	Login string `json:"login"`
}

type apiLabel struct {
	Name string `json:"name"`
}

type apiMilestone struct {
	Title string `json:"title"`
}

// isPullRequest reports whether the shared-API record is a pull request.
func (issue apiIssue) isPullRequest() bool {
	trimmed := strings.TrimSpace(string(issue.Pull))
	return trimmed != "" && trimmed != "null"
}

// FetchAll retrieves every open and closed issue in the repository with
// pagination, excluding pull requests, and returns normalized records.
func (client *Client) FetchAll(ctx context.Context, repository Repository) ([]Issue, error) {
	var collected []Issue
	seen := make(map[int]bool)

	for page := 1; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("fetch %s issues: %w", repository.Slug(), err)
		}

		var records []apiIssue
		if err := client.api.DoWithContext(ctx, "GET", issuesPath(repository, page), nil, &records); err != nil {
			return nil, fmt.Errorf("fetch %s issues page %d: %w", repository.Slug(), page, err)
		}

		for _, record := range records {
			if record.isPullRequest() {
				continue
			}
			if seen[record.Number] {
				return nil, fmt.Errorf("fetch %s issues: duplicate issue number %d", repository.Slug(), record.Number)
			}
			seen[record.Number] = true
			collected = append(collected, normalizeIssue(record))
		}

		if len(records) < issuesPageSize {
			return collected, nil
		}
	}
}

// issuesPath builds the escaped REST path and query for one issues page.
func issuesPath(repository Repository, page int) string {
	query := url.Values{}
	query.Set("state", "all")
	query.Set("per_page", fmt.Sprintf("%d", issuesPageSize))
	query.Set("page", fmt.Sprintf("%d", page))
	query.Set("sort", "created")
	query.Set("direction", "asc")
	return fmt.Sprintf(
		"repos/%s/%s/issues?%s",
		url.PathEscape(repository.Owner),
		url.PathEscape(repository.Name),
		query.Encode(),
	)
}

// normalizeIssue converts a wire record into the normalized issue model.
func normalizeIssue(record apiIssue) Issue {
	labels := make([]string, 0, len(record.Labels))
	for _, label := range record.Labels {
		labels = append(labels, label.Name)
	}
	assignees := make([]string, 0, len(record.Assignees))
	for _, assignee := range record.Assignees {
		assignees = append(assignees, assignee.Login)
	}

	body := ""
	if record.Body != nil {
		body = *record.Body
	}
	var milestone *string
	if record.Milestone != nil {
		title := record.Milestone.Title
		milestone = &title
	}

	return Issue{
		Number:    record.Number,
		Title:     record.Title,
		Body:      body,
		State:     IssueState(strings.ToLower(record.State)),
		Author:    record.User.Login,
		URL:       record.HTMLURL,
		Labels:    normalizeNames(labels),
		Assignees: normalizeNames(assignees),
		Milestone: milestone,
		CreatedAt: record.CreatedAt,
		UpdatedAt: record.UpdatedAt,
	}
}

// ViewerLogin returns the authenticated user's login for "@me" resolution.
func (client *Client) ViewerLogin(ctx context.Context) (string, error) {
	var viewer apiUser
	if err := client.api.DoWithContext(ctx, "GET", "user", nil, &viewer); err != nil {
		return "", fmt.Errorf("resolve authenticated user: %w", err)
	}
	login := strings.TrimSpace(viewer.Login)
	if login == "" {
		return "", fmt.Errorf("resolve authenticated user: empty login")
	}
	return login, nil
}
