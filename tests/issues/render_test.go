package issues_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/digimata/gh-render/internal/issues"
)

var updateGoldens = flag.Bool("update", false, "rewrite golden files under testdata/")

// assertGolden compares got with the named golden file, rewriting it first
// when the -update flag is set.
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGoldens {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("update golden %s: %v", path, err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to generate): %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

var renderRepository = issues.Repository{Host: "github.com", Owner: "octo", Name: "repo"}

func stringPointer(value string) *string {
	return &value
}

// fullIssue mirrors the docs/objects/issues.md section 6 example with every
// field populated.
func fullIssue() issues.Issue {
	return issues.Issue{
		Number:    7,
		Title:     "Make file imports atomically non-overwriting",
		Body:      "Imports must never clobber files.\n\n- write to a temporary file\n- rename into place\n",
		State:     issues.IssueOpen,
		Author:    "dremnik",
		URL:       "https://github.com/octo/repo/issues/7",
		Labels:    []string{"bug", "p0"},
		Assignees: []string{"dremnik"},
		Milestone: stringPointer("v1.0"),
		CreatedAt: time.Date(2026, 7, 26, 19, 37, 25, 0, time.UTC),
		UpdatedAt: time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC),
	}
}

// minimalIssue has an empty body and no labels, assignees, or milestone.
func minimalIssue() issues.Issue {
	return issues.Issue{
		Number:    12,
		Title:     "Placeholder",
		State:     issues.IssueClosed,
		Author:    "octocat",
		URL:       "https://github.com/octo/repo/issues/12",
		CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}
}

// specialIssue stresses quoting: quotes, backslashes, pipes, Unicode, and
// backticks in the title and labels, CRLF and trailing blanks in the body.
func specialIssue() issues.Issue {
	return issues.Issue{
		Number: 33,
		Title:  "He said \"hi\" \\ C:\\path | naïve `code` — ✓",
		Body: "Line one\r\nLine | two \"quoted\" \\backslash\r\rUnicode: naïve — ✓\n" +
			"```go\nfmt.Println(\"hé\")\n```\n\n   \n\t\n",
		State:     issues.IssueOpen,
		Author:    "octocat",
		URL:       "https://github.com/octo/repo/issues/33",
		Labels:    []string{"needs `review`", "pipe|label", "quote\"label"},
		Assignees: []string{"naïve-user"},
		CreatedAt: time.Date(2026, 3, 14, 15, 9, 26, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC),
	}
}

func TestGoldenIssueFile(t *testing.T) {
	tests := []struct {
		golden string
		issue  issues.Issue
	}{
		{"issue.golden.md", fullIssue()},
		{"issue-minimal.golden.md", minimalIssue()},
		{"issue-special.golden.md", specialIssue()},
	}
	for _, test := range tests {
		t.Run(test.golden, func(t *testing.T) {
			content, err := issues.RenderIssue(test.issue)
			if err != nil {
				t.Fatalf("RenderIssue: %v", err)
			}
			assertGolden(t, test.golden, content)
			if !issues.IsManagedContent(content) {
				t.Error("rendered issue must carry the ownership marker")
			}
		})
	}
}

func TestGoldenIndexFull(t *testing.T) {
	selection := baseSelection()
	content, err := issues.RenderIndex(
		renderRepository,
		selection,
		[]issues.Issue{fullIssue(), minimalIssue(), specialIssue()},
	)
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	assertGolden(t, "index.golden.md", content)
	if !issues.IsManagedContent(content) {
		t.Error("rendered index must carry the ownership marker")
	}
}

func TestGoldenIndexFiltered(t *testing.T) {
	selection := issues.ResolvedSelection{
		State:    issues.StateOpen,
		Labels:   []string{"bug", "p0"},
		Assignee: "dremnik",
		Limit:    20,
		Sort:     issues.SortUpdated,
		Order:    issues.OrderDescending,
	}
	content, err := issues.RenderIndex(renderRepository, selection, []issues.Issue{fullIssue()})
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	assertGolden(t, "index-filtered.golden.md", content)
	if bytes.Contains(content, []byte("@me")) {
		t.Error("index must contain the concrete login, never @me")
	}
	if !bytes.Contains(content, []byte("assignee=dremnik")) {
		t.Error("index selection line missing concrete assignee")
	}
}

func TestRenderFileNamesAndOrder(t *testing.T) {
	long := minimalIssue()
	long.Number = 12345
	files, err := issues.Render(renderRepository, baseSelection(), []issues.Issue{fullIssue(), long})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = file.Name
	}
	// Numbers over four digits are not truncated; files sort lexically.
	if !reflect.DeepEqual(names, []string{"index.md", "iss-0007.md", "iss-12345.md"}) {
		t.Errorf("names = %v", names)
	}
	if !bytes.Contains(files[2].Content, []byte("# ISS-12345 — Placeholder")) {
		t.Errorf("long-number heading missing:\n%s", files[2].Content)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	selected := []issues.Issue{fullIssue(), minimalIssue(), specialIssue()}
	first, err := issues.Render(renderRepository, baseSelection(), selected)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	second, err := issues.Render(renderRepository, baseSelection(), selected)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("two renders of the same input produced different bytes")
	}
}

func TestRenderIssueBodyNormalization(t *testing.T) {
	issue := minimalIssue()
	issue.Body = "alpha\r\nbeta\r\rgamma\n\n   \n\t\n"

	content, err := issues.RenderIssue(issue)
	if err != nil {
		t.Fatalf("RenderIssue: %v", err)
	}
	// CRLF and CR become LF, trailing blank lines fall away, one final newline.
	if !bytes.HasSuffix(content, []byte("alpha\nbeta\n\ngamma\n")) {
		t.Errorf("body not normalized:\n%q", content)
	}
	if bytes.HasSuffix(content, []byte("\n\n")) || bytes.Contains(content, []byte("\r")) {
		t.Errorf("residual CR or trailing blank line:\n%q", content)
	}
}

func TestRenderIssueEmptyBodyEndsAfterLink(t *testing.T) {
	content, err := issues.RenderIssue(minimalIssue())
	if err != nil {
		t.Fatalf("RenderIssue: %v", err)
	}
	if !bytes.HasSuffix(content, []byte("GitHub: [#12](https://github.com/octo/repo/issues/12)\n")) {
		t.Errorf("empty-body file must end at the GitHub link:\n%q", content)
	}
}

func TestIsManagedName(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"index.md", true},
		{"iss-0001.md", true},
		{"iss-0007.md", true},
		{"iss-12345.md", true},
		{"iss-001.md", false},   // fewer than four digits
		{"iss-0001.txt", false}, // wrong extension
		{"iss-0001.md.bak", false},
		{"Iss-0001.md", false}, // pattern is case-sensitive
		{"INDEX.md", false},
		{"iss-.md", false},
		{"iss-0001a.md", false},
		{"README.md", false},
		{"notes.md", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := issues.IsManagedName(test.name); got != test.expected {
				t.Errorf("IsManagedName(%q) = %v, want %v", test.name, got, test.expected)
			}
		})
	}
}

func TestIsManagedContent(t *testing.T) {
	marker := "<!-- gh-render:managed -->"
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"marker at byte zero", marker + "\n\n# Issues\n", true},
		{"marker after frontmatter", "---\nid: 7\n---\n" + marker + "\n\n# ISS-0007 — Title\n", true},
		{"marker on CRLF line", "---\r\nid: 7\r\n---\r\n" + marker + "\r\n\r\n# ISS-0007 — Title\r\n", true},
		{"marker without any heading", marker + "\nno headings here\n", true},
		{"marker embedded in prose before heading", "This note mentions " + marker + " in prose.\n\n# Personal notes\n", false},
		{"marker embedded in frontmatter", "---\ntitle: \"A " + marker + " mention\"\n---\n\n# Personal notes\n", false},
		{"marker with leading whitespace", "  " + marker + "\n\n# Personal notes\n", false},
		{"marker with trailing text", marker + " not really\n\n# Personal notes\n", false},
		{"marker only inside user body", "# My notes\n\nQuoting " + marker + " here.\n", false},
		{"no marker", "# Issues\n\nplain file\n", false},
		{"empty", "", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := issues.IsManagedContent([]byte(test.content)); got != test.expected {
				t.Errorf("IsManagedContent = %v, want %v", got, test.expected)
			}
		})
	}
}

func TestIndexLabelsRenderInStableOrder(t *testing.T) {
	// Issue labels are normalized (deduplicated, bytewise-sorted) at the API
	// boundary; the index renders them in exactly that order as inline code.
	issue := minimalIssue()
	issue.Labels = []string{"Alpha", "beta", "gamma"}

	content, err := issues.RenderIndex(renderRepository, baseSelection(), []issues.Issue{issue})
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	if !bytes.Contains(content, []byte("`Alpha`, `beta`, `gamma`")) {
		t.Errorf("labels cell out of order:\n%s", content)
	}
}
