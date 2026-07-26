package issues_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/digimata/gh-render/internal/issues"
)

// baseSelection returns a valid unfiltered resolved selection to mutate.
func baseSelection() issues.ResolvedSelection {
	return issues.ResolvedSelection{
		State: issues.StateAll,
		Sort:  issues.SortUpdated,
		Order: issues.OrderDescending,
	}
}

// validSelection returns a valid unresolved selection to mutate.
func validSelection() issues.Selection {
	return issues.Selection{
		State: issues.StateAll,
		Sort:  issues.SortUpdated,
		Order: issues.OrderDescending,
	}
}

// filterSource covers state, label, assignee, and author variety, including
// spellings that differ only by case.
func filterSource() []issues.Issue {
	at := func(day int) time.Time {
		return time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC)
	}
	return []issues.Issue{
		{Number: 1, State: issues.IssueOpen, Labels: []string{"bug"}, Assignees: []string{"alice"}, Author: "alice", UpdatedAt: at(1)},
		{Number: 2, State: issues.IssueClosed, Labels: []string{"bug", "p0"}, Assignees: []string{"bob"}, Author: "alice", UpdatedAt: at(2)},
		{Number: 3, State: issues.IssueOpen, Labels: []string{"p0"}, Assignees: []string{"alice", "bob"}, Author: "bob", UpdatedAt: at(3)},
		{Number: 4, State: issues.IssueClosed, Author: "carol", UpdatedAt: at(4)},
		{Number: 5, State: issues.IssueOpen, Labels: []string{"Bug"}, Assignees: []string{"ALICE"}, Author: "Alice", UpdatedAt: at(5)},
	}
}

func selectedNumbers(t *testing.T, source []issues.Issue, selection issues.ResolvedSelection) []int {
	t.Helper()
	selected := issues.Select(source, selection)
	numbers := make([]int, len(selected))
	for i, issue := range selected {
		numbers[i] = issue.Number
	}
	return numbers
}

func TestSelectFiltering(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*issues.ResolvedSelection)
		expected []int
	}{
		{"state all", func(*issues.ResolvedSelection) {}, []int{1, 2, 3, 4, 5}},
		{"state open", func(s *issues.ResolvedSelection) { s.State = issues.StateOpen }, []int{1, 3, 5}},
		{"state closed", func(s *issues.ResolvedSelection) { s.State = issues.StateClosed }, []int{2, 4}},
		{"one label", func(s *issues.ResolvedSelection) { s.Labels = []string{"bug"} }, []int{1, 2, 5}},
		{"repeated labels require every label", func(s *issues.ResolvedSelection) {
			s.Labels = []string{"bug", "p0"}
		}, []int{2}},
		{"assignee", func(s *issues.ResolvedSelection) { s.Assignee = "alice" }, []int{1, 3, 5}},
		{"author", func(s *issues.ResolvedSelection) { s.Author = "alice" }, []int{1, 2, 5}},
		{"label case-insensitive", func(s *issues.ResolvedSelection) { s.Labels = []string{"BUG"} }, []int{1, 2, 5}},
		{"assignee case-insensitive", func(s *issues.ResolvedSelection) { s.Assignee = "Alice" }, []int{1, 3, 5}},
		{"author case-insensitive", func(s *issues.ResolvedSelection) { s.Author = "ALICE" }, []int{1, 2, 5}},
		{"state and label", func(s *issues.ResolvedSelection) {
			s.State = issues.StateOpen
			s.Labels = []string{"bug"}
		}, []int{1, 5}},
		{"state and assignee", func(s *issues.ResolvedSelection) {
			s.State = issues.StateClosed
			s.Assignee = "bob"
		}, []int{2}},
		{"state and author", func(s *issues.ResolvedSelection) {
			s.State = issues.StateClosed
			s.Author = "alice"
		}, []int{2}},
		{"label and assignee", func(s *issues.ResolvedSelection) {
			s.Labels = []string{"p0"}
			s.Assignee = "bob"
		}, []int{2, 3}},
		{"label and author", func(s *issues.ResolvedSelection) {
			s.Labels = []string{"bug"}
			s.Author = "alice"
		}, []int{1, 2, 5}},
		{"assignee and author", func(s *issues.ResolvedSelection) {
			s.Assignee = "alice"
			s.Author = "bob"
		}, []int{3}},
		{"every selector combined", func(s *issues.ResolvedSelection) {
			s.State = issues.StateOpen
			s.Labels = []string{"p0"}
			s.Assignee = "bob"
			s.Author = "bob"
		}, []int{3}},
		{"no match", func(s *issues.ResolvedSelection) {
			s.Labels = []string{"bug"}
			s.Author = "carol"
		}, []int{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := baseSelection()
			test.mutate(&selection)
			numbers := selectedNumbers(t, filterSource(), selection)
			if !reflect.DeepEqual(numbers, test.expected) {
				t.Errorf("selected = %v, want %v", numbers, test.expected)
			}
		})
	}
}

// rankingSource gives every sort field a distinct winner: issue 1 is the most
// recently updated, issue 3 the most recently created and highest numbered.
func rankingSource() []issues.Issue {
	at := func(day int) time.Time {
		return time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC)
	}
	return []issues.Issue{
		{Number: 1, State: issues.IssueOpen, CreatedAt: at(1), UpdatedAt: at(13)},
		{Number: 2, State: issues.IssueOpen, CreatedAt: at(2), UpdatedAt: at(12)},
		{Number: 3, State: issues.IssueOpen, CreatedAt: at(3), UpdatedAt: at(11)},
	}
}

func TestSelectRankingAndLimit(t *testing.T) {
	tests := []struct {
		name     string
		sort     issues.SortField
		order    issues.SortOrder
		limit    int
		expected []int
	}{
		{"updated desc keeps newest", issues.SortUpdated, issues.OrderDescending, 1, []int{1}},
		{"updated asc keeps oldest", issues.SortUpdated, issues.OrderAscending, 1, []int{3}},
		{"created desc keeps newest", issues.SortCreated, issues.OrderDescending, 1, []int{3}},
		{"created asc keeps oldest", issues.SortCreated, issues.OrderAscending, 1, []int{1}},
		{"number desc keeps highest", issues.SortNumber, issues.OrderDescending, 1, []int{3}},
		{"number asc keeps lowest", issues.SortNumber, issues.OrderAscending, 1, []int{1}},
		{"limit larger than matches", issues.SortUpdated, issues.OrderDescending, 10, []int{1, 2, 3}},
		{"unlimited", issues.SortUpdated, issues.OrderDescending, 0, []int{1, 2, 3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := baseSelection()
			selection.Sort = test.sort
			selection.Order = test.order
			selection.Limit = test.limit
			numbers := selectedNumbers(t, rankingSource(), selection)
			if !reflect.DeepEqual(numbers, test.expected) {
				t.Errorf("selected = %v, want %v", numbers, test.expected)
			}
		})
	}
}

func TestSelectEqualTimestampsBreakTiesByNumber(t *testing.T) {
	moment := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	source := []issues.Issue{
		{Number: 5, State: issues.IssueOpen, CreatedAt: moment, UpdatedAt: moment},
		{Number: 9, State: issues.IssueOpen, CreatedAt: moment, UpdatedAt: moment},
		{Number: 2, State: issues.IssueOpen, CreatedAt: moment, UpdatedAt: moment},
	}

	selection := baseSelection()
	selection.Limit = 2
	if numbers := selectedNumbers(t, source, selection); !reflect.DeepEqual(numbers, []int{5, 9}) {
		t.Errorf("descending tie-break selected %v, want [5 9]", numbers)
	}

	selection.Order = issues.OrderAscending
	if numbers := selectedNumbers(t, source, selection); !reflect.DeepEqual(numbers, []int{2, 5}) {
		t.Errorf("ascending tie-break selected %v, want [2 5]", numbers)
	}
}

func TestSelectLimitAppliesBeforeSerializationOrder(t *testing.T) {
	at := func(day int) time.Time {
		return time.Date(2026, 7, day, 0, 0, 0, 0, time.UTC)
	}
	source := []issues.Issue{
		{Number: 7, State: issues.IssueOpen, UpdatedAt: at(1)},
		{Number: 2, State: issues.IssueOpen, UpdatedAt: at(2)},
		{Number: 10, State: issues.IssueOpen, UpdatedAt: at(3)},
	}

	// Ranking keeps 10 and 2 (most recently updated); serialization then
	// orders the survivors by ascending number.
	selection := baseSelection()
	selection.Limit = 2
	if numbers := selectedNumbers(t, source, selection); !reflect.DeepEqual(numbers, []int{2, 10}) {
		t.Errorf("selected = %v, want [2 10]", numbers)
	}
}

func TestSelectDoesNotMutateSource(t *testing.T) {
	source := rankingSource()
	source[0], source[2] = source[2], source[0] // deliberately unsorted
	snapshot := make([]issues.Issue, len(source))
	copy(snapshot, source)

	selection := baseSelection()
	selection.Limit = 1
	issues.Select(source, selection)
	if !reflect.DeepEqual(source, snapshot) {
		t.Errorf("source mutated: %v", source)
	}
}

func TestNeedsViewer(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*issues.Selection)
		expected bool
	}{
		{"no selectors", func(*issues.Selection) {}, false},
		{"concrete assignee", func(s *issues.Selection) { s.Assignee = "octocat" }, false},
		{"concrete author", func(s *issues.Selection) { s.Author = "octocat" }, false},
		{"assignee alias", func(s *issues.Selection) { s.Assignee = "@me" }, true},
		{"author alias", func(s *issues.Selection) { s.Author = "@me" }, true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := validSelection()
			test.mutate(&selection)
			if got := issues.NeedsViewer(selection); got != test.expected {
				t.Errorf("NeedsViewer = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestResolveSelectionReplacesAlias(t *testing.T) {
	selection := validSelection()
	selection.Assignee = "@me"
	selection.Author = "@me"

	resolved, err := issues.ResolveSelection(selection, "octocat")
	if err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if resolved.Assignee != "octocat" || resolved.Author != "octocat" {
		t.Errorf("resolved = %+v, want octocat for both logins", resolved)
	}
}

func TestResolveSelectionKeepsConcreteLoginsWithoutViewer(t *testing.T) {
	selection := validSelection()
	selection.Assignee = "alice"
	selection.Author = "bob"

	resolved, err := issues.ResolveSelection(selection, "")
	if err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if resolved.Assignee != "alice" || resolved.Author != "bob" {
		t.Errorf("resolved = %+v", resolved)
	}
}

func TestResolveSelectionRejectsAliasWithoutViewer(t *testing.T) {
	for _, field := range []string{"assignee", "author"} {
		t.Run(field, func(t *testing.T) {
			selection := validSelection()
			if field == "assignee" {
				selection.Assignee = "@me"
			} else {
				selection.Author = "@me"
			}
			_, err := issues.ResolveSelection(selection, "")
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Errorf("error = %v, want %s alias rejection", err, field)
			}
		})
	}
}

func TestResolveSelectionValidation(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*issues.Selection)
		fragment string
	}{
		{"invalid state", func(s *issues.Selection) { s.State = "merged" }, "invalid state"},
		{"invalid sort", func(s *issues.Selection) { s.Sort = "priority" }, "invalid sort"},
		{"invalid order", func(s *issues.Selection) { s.Order = "up" }, "invalid order"},
		{"negative limit", func(s *issues.Selection) { s.Limit = -1 }, "invalid limit"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			selection := validSelection()
			test.mutate(&selection)
			_, err := issues.ResolveSelection(selection, "octocat")
			if err == nil || !strings.Contains(err.Error(), test.fragment) {
				t.Errorf("error = %v, want %q", err, test.fragment)
			}
		})
	}
}

func TestResolveSelectionNormalizesLabels(t *testing.T) {
	selection := validSelection()
	selection.Labels = []string{"p0", "Bug", "bug", "alpha"}
	original := make([]string, len(selection.Labels))
	copy(original, selection.Labels)

	resolved, err := issues.ResolveSelection(selection, "")
	if err != nil {
		t.Fatalf("ResolveSelection: %v", err)
	}
	if !reflect.DeepEqual(resolved.Labels, []string{"Bug", "alpha", "p0"}) {
		t.Errorf("labels = %v, want [Bug alpha p0]", resolved.Labels)
	}
	if !reflect.DeepEqual(selection.Labels, original) {
		t.Errorf("input labels mutated: %v", selection.Labels)
	}
}
