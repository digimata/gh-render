// Package issues implements the gh-render issues object: fetching,
// normalizing, selecting, and rendering GitHub issues as a deterministic
// Markdown projection.
package issues

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Repository identifies a GitHub repository on a specific host.
type Repository struct {
	Host  string
	Owner string
	Name  string
}

// Slug returns the owner/name form of the repository.
func (repository Repository) Slug() string {
	return repository.Owner + "/" + repository.Name
}

// IssuesURL returns the browser URL of the repository's issue list.
func (repository Repository) IssuesURL() string {
	host := repository.Host
	if host == "" {
		host = "github.com"
	}
	return fmt.Sprintf("https://%s/%s/%s/issues", host, repository.Owner, repository.Name)
}

// IssueState is the normalized lowercase state of an issue.
type IssueState string

const (
	IssueOpen   IssueState = "open"
	IssueClosed IssueState = "closed"
)

// StateFilter selects issues by state.
type StateFilter string

const (
	StateAll    StateFilter = "all"
	StateOpen   StateFilter = "open"
	StateClosed StateFilter = "closed"
)

// SortField ranks matches before the limit is applied.
type SortField string

const (
	SortUpdated SortField = "updated"
	SortCreated SortField = "created"
	SortNumber  SortField = "number"
)

// SortOrder is the ranking direction.
type SortOrder string

const (
	OrderAscending  SortOrder = "asc"
	OrderDescending SortOrder = "desc"
)

// Issue is the normalized issue model rendered into the projection. It is
// independent of GitHub API response structs.
type Issue struct {
	Number    int
	Title     string
	Body      string
	State     IssueState
	Author    string
	URL       string
	Labels    []string
	Assignees []string
	Milestone *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Selection is the user-supplied selection. Assignee and Author may contain
// the "@me" alias; empty means any. A zero limit means unlimited.
type Selection struct {
	State    StateFilter
	Labels   []string
	Assignee string
	Author   string
	Limit    int
	Sort     SortField
	Order    SortOrder
}

// ResolvedSelection is a Selection whose aliases have been replaced with
// concrete GitHub logins and whose labels are deduplicated and sorted.
type ResolvedSelection struct {
	State    StateFilter
	Labels   []string
	Assignee string
	Author   string
	Limit    int
	Sort     SortField
	Order    SortOrder
}

// normalizeNames removes case-insensitive duplicates, keeping the first
// spelling, and sorts the preserved spellings with bytewise lexical ordering.
func normalizeNames(values []string) []string {
	normalized := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		normalized = append(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}
