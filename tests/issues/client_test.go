package issues_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/digimata/gh-render/internal/issues"
)

// fakeREST is a canned-response issues.RESTDoer. It records every call and
// fills the response argument by unmarshaling the configured JSON body.
type fakeREST struct {
	responses map[string]string
	failures  map[string]error
	calls     []string
}

func (fake *fakeREST) DoWithContext(
	_ context.Context,
	method string,
	path string,
	_ io.Reader,
	response any,
) error {
	fake.calls = append(fake.calls, method+" "+path)
	if err, ok := fake.failures[path]; ok {
		return err
	}
	body, ok := fake.responses[path]
	if !ok {
		return fmt.Errorf("fakeREST: unexpected path %q", path)
	}
	return json.Unmarshal([]byte(body), response)
}

var testRepository = issues.Repository{Host: "github.com", Owner: "octo", Name: "repo"}

// issuesPagePath is the exact request path the client must build for a page.
// url.Values.Encode sorts query keys.
func issuesPagePath(repository issues.Repository, page int) string {
	return fmt.Sprintf(
		"repos/%s/%s/issues?direction=asc&page=%d&per_page=100&sort=created&state=all",
		repository.Owner, repository.Name, page,
	)
}

// issueRecord returns one wire-shaped issue record; tests mutate the map.
func issueRecord(number int) map[string]any {
	return map[string]any{
		"number":     number,
		"title":      fmt.Sprintf("Issue %d", number),
		"body":       fmt.Sprintf("Body %d", number),
		"state":      "open",
		"html_url":   fmt.Sprintf("https://github.com/octo/repo/issues/%d", number),
		"user":       map[string]any{"login": "octocat"},
		"labels":     []any{},
		"assignees":  []any{},
		"milestone":  nil,
		"created_at": "2026-07-01T00:00:00Z",
		"updated_at": "2026-07-02T00:00:00Z",
	}
}

// pageBody marshals records into one JSON response page.
func pageBody(t *testing.T, records []map[string]any) string {
	t.Helper()
	body, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal page: %v", err)
	}
	return string(body)
}

// recordRange builds sequential records covering [first, first+count).
func recordRange(first, count int) []map[string]any {
	records := make([]map[string]any, 0, count)
	for number := first; number < first+count; number++ {
		records = append(records, issueRecord(number))
	}
	return records
}

func issueNumbers(collected []issues.Issue) []int {
	numbers := make([]int, len(collected))
	for i, issue := range collected {
		numbers[i] = issue.Number
	}
	return numbers
}

func TestFetchAllRequestEncoding(t *testing.T) {
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, recordRange(1, 1)),
	}}

	if _, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository); err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	expected := []string{
		"GET repos/octo/repo/issues?direction=asc&page=1&per_page=100&sort=created&state=all",
	}
	if len(fake.calls) != 1 || fake.calls[0] != expected[0] {
		t.Errorf("calls = %v, want %v", fake.calls, expected)
	}
}

func TestFetchAllEscapesPathSegments(t *testing.T) {
	spaced := issues.Repository{Host: "github.com", Owner: "octo cat", Name: "re po"}
	path := "repos/octo%20cat/re%20po/issues?direction=asc&page=1&per_page=100&sort=created&state=all"
	fake := &fakeREST{responses: map[string]string{path: "[]"}}

	if _, err := issues.NewClient(fake).FetchAll(context.Background(), spaced); err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "GET "+path {
		t.Errorf("calls = %v, want [GET %s]", fake.calls, path)
	}
}

func TestFetchAllSinglePage(t *testing.T) {
	records := recordRange(1, 3)
	records[1]["state"] = "CLOSED"
	records[2]["pull_request"] = nil // explicit null is not a pull request
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, records),
	}}

	collected, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if numbers := issueNumbers(collected); fmt.Sprint(numbers) != "[1 2 3]" {
		t.Fatalf("numbers = %v, want [1 2 3]", numbers)
	}
	first := collected[0]
	if first.Title != "Issue 1" || first.Body != "Body 1" || first.Author != "octocat" {
		t.Errorf("issue 1 = %+v", first)
	}
	if first.URL != "https://github.com/octo/repo/issues/1" {
		t.Errorf("URL = %q", first.URL)
	}
	if first.CreatedAt.Format("2006-01-02") != "2026-07-01" {
		t.Errorf("created at = %v", first.CreatedAt)
	}
	if collected[1].State != issues.IssueClosed {
		t.Errorf("state = %q, want normalized %q", collected[1].State, issues.IssueClosed)
	}
	if len(fake.calls) != 1 {
		t.Errorf("calls = %v, want a single page", fake.calls)
	}
}

func TestFetchAllMultiplePages(t *testing.T) {
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, recordRange(1, 100)),
		issuesPagePath(testRepository, 2): pageBody(t, recordRange(101, 2)),
	}}

	collected, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(collected) != 102 {
		t.Errorf("collected %d issues, want 102", len(collected))
	}
	if len(fake.calls) != 2 {
		t.Errorf("calls = %v, want two pages", fake.calls)
	}
}

func TestFetchAllExactlyFullFinalPage(t *testing.T) {
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, recordRange(1, 100)),
		issuesPagePath(testRepository, 2): "[]",
	}}

	collected, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(collected) != 100 {
		t.Errorf("collected %d issues, want 100", len(collected))
	}
	if len(fake.calls) != 2 {
		t.Errorf("calls = %v, want one extra empty page", fake.calls)
	}
}

func TestFetchAllPullRequestsDoNotStopPagination(t *testing.T) {
	// A full raw page that normalizes to fewer than 100 issues must still
	// trigger the next request: termination follows the raw page length.
	first := recordRange(1, 100)
	for i := 0; i < 40; i++ {
		first[i]["pull_request"] = map[string]any{
			"url": fmt.Sprintf("https://api.github.com/repos/octo/repo/pulls/%d", i+1),
		}
	}
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, first),
		issuesPagePath(testRepository, 2): pageBody(t, recordRange(101, 1)),
	}}

	collected, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("calls = %v, want two pages despite pull requests", fake.calls)
	}
	if len(collected) != 61 {
		t.Errorf("collected %d issues, want 61", len(collected))
	}
	for _, issue := range collected {
		if issue.Number <= 40 {
			t.Errorf("pull request %d leaked into issues", issue.Number)
		}
	}
}

func TestFetchAllNullBodyAndMilestone(t *testing.T) {
	records := recordRange(1, 2)
	records[0]["body"] = nil
	records[0]["milestone"] = nil
	records[1]["milestone"] = map[string]any{"title": "v1.0"}
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, records),
	}}

	collected, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if collected[0].Body != "" {
		t.Errorf("null body = %q, want empty string", collected[0].Body)
	}
	if collected[0].Milestone != nil {
		t.Errorf("null milestone = %v, want nil", *collected[0].Milestone)
	}
	if collected[1].Milestone == nil || *collected[1].Milestone != "v1.0" {
		t.Errorf("milestone = %v, want v1.0", collected[1].Milestone)
	}
}

func TestFetchAllNormalizesLabelsAndAssignees(t *testing.T) {
	record := issueRecord(1)
	record["labels"] = []any{
		map[string]any{"name": "beta"},
		map[string]any{"name": "Alpha"},
		map[string]any{"name": "ALPHA"}, // case-insensitive duplicate
	}
	record["assignees"] = []any{
		map[string]any{"login": "Zed"},
		map[string]any{"login": "zed"},
		map[string]any{"login": "abe"},
	}
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, []map[string]any{record}),
	}}

	collected, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	// First spelling survives deduplication; bytewise sort puts uppercase first.
	if fmt.Sprint(collected[0].Labels) != "[Alpha beta]" {
		t.Errorf("labels = %v, want [Alpha beta]", collected[0].Labels)
	}
	if fmt.Sprint(collected[0].Assignees) != "[Zed abe]" {
		t.Errorf("assignees = %v, want [Zed abe]", collected[0].Assignees)
	}
}

func TestFetchAllRejectsDuplicateNumbersAcrossPages(t *testing.T) {
	second := []map[string]any{issueRecord(100)}
	fake := &fakeREST{responses: map[string]string{
		issuesPagePath(testRepository, 1): pageBody(t, recordRange(1, 100)),
		issuesPagePath(testRepository, 2): pageBody(t, second),
	}}

	_, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if err == nil || !strings.Contains(err.Error(), "duplicate issue number 100") {
		t.Errorf("error = %v, want duplicate issue number 100", err)
	}
}

func TestFetchAllWrapsRESTError(t *testing.T) {
	cause := errors.New("HTTP 502")
	fake := &fakeREST{failures: map[string]error{
		issuesPagePath(testRepository, 1): cause,
	}}

	_, err := issues.NewClient(fake).FetchAll(context.Background(), testRepository)
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v, want wrapped %v", err, cause)
	}
	if !strings.Contains(err.Error(), "fetch octo/repo issues page 1") {
		t.Errorf("error = %v, want operation and subject context", err)
	}
}

func TestFetchAllCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &fakeREST{}

	_, err := issues.NewClient(fake).FetchAll(ctx, testRepository)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want %v", err, context.Canceled)
	}
	if len(fake.calls) != 0 {
		t.Errorf("calls = %v, want none after cancellation", fake.calls)
	}
}

func TestViewerLogin(t *testing.T) {
	fake := &fakeREST{responses: map[string]string{"user": `{"login":"octocat"}`}}

	login, err := issues.NewClient(fake).ViewerLogin(context.Background())
	if err != nil {
		t.Fatalf("ViewerLogin: %v", err)
	}
	if login != "octocat" {
		t.Errorf("login = %q, want octocat", login)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "GET user" {
		t.Errorf("calls = %v, want [GET user]", fake.calls)
	}
}

func TestViewerLoginRejectsEmptyLogin(t *testing.T) {
	fake := &fakeREST{responses: map[string]string{"user": `{"login":"  "}`}}

	_, err := issues.NewClient(fake).ViewerLogin(context.Background())
	if err == nil || !strings.Contains(err.Error(), "empty login") {
		t.Errorf("error = %v, want empty-login rejection", err)
	}
}
