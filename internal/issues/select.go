package issues

import (
	"cmp"
	"fmt"
	"sort"
	"strings"
)

const viewerAlias = "@me"

// NeedsViewer reports whether resolving the selection requires the
// authenticated viewer's login.
func NeedsViewer(selection Selection) bool {
	return selection.Assignee == viewerAlias || selection.Author == viewerAlias
}

// ResolveSelection validates the selection, replaces "@me" with the viewer
// login, and normalizes labels. It does not mutate its input.
func ResolveSelection(selection Selection, viewerLogin string) (ResolvedSelection, error) {
	switch selection.State {
	case StateAll, StateOpen, StateClosed:
	default:
		return ResolvedSelection{}, fmt.Errorf("invalid state %q", selection.State)
	}
	switch selection.Sort {
	case SortUpdated, SortCreated, SortNumber:
	default:
		return ResolvedSelection{}, fmt.Errorf("invalid sort field %q", selection.Sort)
	}
	switch selection.Order {
	case OrderAscending, OrderDescending:
	default:
		return ResolvedSelection{}, fmt.Errorf("invalid order %q", selection.Order)
	}
	if selection.Limit < 0 {
		return ResolvedSelection{}, fmt.Errorf("invalid limit %d", selection.Limit)
	}

	assignee, err := resolveLogin(selection.Assignee, viewerLogin)
	if err != nil {
		return ResolvedSelection{}, fmt.Errorf("resolve assignee: %w", err)
	}
	author, err := resolveLogin(selection.Author, viewerLogin)
	if err != nil {
		return ResolvedSelection{}, fmt.Errorf("resolve author: %w", err)
	}

	return ResolvedSelection{
		State:    selection.State,
		Labels:   normalizeNames(selection.Labels),
		Assignee: assignee,
		Author:   author,
		Limit:    selection.Limit,
		Sort:     selection.Sort,
		Order:    selection.Order,
	}, nil
}

func resolveLogin(value, viewerLogin string) (string, error) {
	if value != viewerAlias {
		return value, nil
	}
	if viewerLogin == "" {
		return "", fmt.Errorf("cannot resolve %q without an authenticated login", viewerAlias)
	}
	return viewerLogin, nil
}

// Select filters, ranks, and limits the source issues, then returns the
// selected subset sorted by ascending issue number for serialization. The
// source slice is never mutated.
func Select(source []Issue, selection ResolvedSelection) []Issue {
	matches := make([]Issue, 0, len(source))
	for _, issue := range source {
		if matchesSelection(issue, selection) {
			matches = append(matches, issue)
		}
	}

	rank(matches, selection.Sort, selection.Order)
	if selection.Limit > 0 && len(matches) > selection.Limit {
		matches = matches[:selection.Limit]
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Number < matches[j].Number
	})
	return matches
}

func matchesSelection(issue Issue, selection ResolvedSelection) bool {
	switch selection.State {
	case StateOpen:
		if issue.State != IssueOpen {
			return false
		}
	case StateClosed:
		if issue.State != IssueClosed {
			return false
		}
	}
	for _, label := range selection.Labels {
		if !containsFold(issue.Labels, label) {
			return false
		}
	}
	if selection.Assignee != "" && !containsFold(issue.Assignees, selection.Assignee) {
		return false
	}
	if selection.Author != "" && !strings.EqualFold(issue.Author, selection.Author) {
		return false
	}
	return true
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

// rank sorts issues in place by the selected field and order, breaking equal
// primary values with an issue-number comparison in the same order.
func rank(issues []Issue, field SortField, order SortOrder) {
	sort.Slice(issues, func(i, j int) bool {
		result := compareIssues(issues[i], issues[j], field)
		if order == OrderDescending {
			return result > 0
		}
		return result < 0
	})
}

func compareIssues(a, b Issue, field SortField) int {
	var primary int
	switch field {
	case SortCreated:
		primary = a.CreatedAt.Compare(b.CreatedAt)
	case SortNumber:
		primary = cmp.Compare(a.Number, b.Number)
	default:
		primary = a.UpdatedAt.Compare(b.UpdatedAt)
	}
	if primary != 0 {
		return primary
	}
	return cmp.Compare(a.Number, b.Number)
}
