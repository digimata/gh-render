package issues

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/digimata/gh-render/internal/projection"
)

var managedIssueName = regexp.MustCompile(`^iss-\d{4,}\.md$`)

// IsManagedName reports whether the file name belongs to the issues renderer.
func IsManagedName(name string) bool {
	return name == "index.md" || managedIssueName.MatchString(name)
}

// IsManagedContent reports whether the content carries the ownership marker
// before its first top-level Markdown heading. A marker appearing only inside
// an issue body never grants ownership.
func IsManagedContent(content []byte) bool {
	markerIndex := bytes.Index(content, []byte(projection.ManagedMarker))
	if markerIndex < 0 {
		return false
	}
	headingIndex := firstHeadingIndex(content)
	return headingIndex < 0 || markerIndex < headingIndex
}

func firstHeadingIndex(content []byte) int {
	offset := 0
	for _, line := range bytes.Split(content, []byte("\n")) {
		if bytes.HasPrefix(line, []byte("# ")) {
			return offset
		}
		offset += len(line) + 1
	}
	return -1
}

// Render returns the complete projection for the selected issues: one index
// file plus one file per issue, sorted lexically by relative name.
func Render(repository Repository, selection ResolvedSelection, selected []Issue) ([]projection.File, error) {
	files := make([]projection.File, 0, len(selected)+1)

	index, err := RenderIndex(repository, selection, selected)
	if err != nil {
		return nil, err
	}
	files = append(files, projection.File{Name: "index.md", Content: index})

	for _, issue := range selected {
		content, err := RenderIssue(issue)
		if err != nil {
			return nil, err
		}
		files = append(files, projection.File{Name: issueFileName(issue.Number), Content: content})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func issueFileName(number int) string {
	return fmt.Sprintf("iss-%04d.md", number)
}

// RenderIssue returns the deterministic Markdown snapshot of one issue.
func RenderIssue(issue Issue) ([]byte, error) {
	var buffer bytes.Buffer

	buffer.WriteString("---\n")
	fmt.Fprintf(&buffer, "id: %d\n", issue.Number)
	fmt.Fprintf(&buffer, "title: %s\n", jsonValue(issue.Title))
	fmt.Fprintf(&buffer, "status: %s\n", issue.State)
	buffer.WriteString("source: github\n")
	fmt.Fprintf(&buffer, "github_url: %s\n", jsonValue(issue.URL))
	fmt.Fprintf(&buffer, "labels: %s\n", jsonArray(issue.Labels))
	fmt.Fprintf(&buffer, "assignees: %s\n", jsonArray(issue.Assignees))
	if issue.Milestone == nil {
		buffer.WriteString("milestone: null\n")
	} else {
		fmt.Fprintf(&buffer, "milestone: %s\n", jsonValue(*issue.Milestone))
	}
	fmt.Fprintf(&buffer, "created_at: %s\n", jsonValue(issue.CreatedAt.UTC().Format(time.RFC3339)))
	fmt.Fprintf(&buffer, "updated_at: %s\n", jsonValue(issue.UpdatedAt.UTC().Format(time.RFC3339)))
	buffer.WriteString("---\n")

	buffer.WriteString(projection.ManagedMarker + "\n\n")
	fmt.Fprintf(&buffer, "# ISS-%04d — %s\n\n", issue.Number, issue.Title)
	fmt.Fprintf(&buffer, "GitHub: [#%d](%s)\n", issue.Number, issue.URL)

	if body := normalizeBody(issue.Body); body != "" {
		buffer.WriteString("\n")
		buffer.WriteString(body)
		buffer.WriteString("\n")
	}
	return buffer.Bytes(), nil
}

// RenderIndex returns the deterministic collection index, including the
// normalized selection line and the issue table sorted by ascending number.
func RenderIndex(repository Repository, selection ResolvedSelection, selected []Issue) ([]byte, error) {
	var buffer bytes.Buffer

	buffer.WriteString(projection.ManagedMarker + "\n\n# Issues\n\n")
	fmt.Fprintf(&buffer, "> Generated from [%s](%s). GitHub is canonical.\n", repository.Slug(), repository.IssuesURL())
	buffer.WriteString(">\n")
	fmt.Fprintf(&buffer, "> Selection: %s.\n\n", selectionSummary(selection))

	buffer.WriteString("| Issue | State | Labels | Updated |\n")
	buffer.WriteString("| --- | --- | --- | --- |\n")
	for _, issue := range selected {
		title := markdownTableCell(fmt.Sprintf("%04d — %s", issue.Number, issue.Title))
		fmt.Fprintf(&buffer, "| [%s](%s) | %s | %s | %s |\n",
			title,
			issueFileName(issue.Number),
			issue.State,
			labelsCell(issue.Labels),
			issue.UpdatedAt.UTC().Format("2006-01-02"),
		)
	}
	return buffer.Bytes(), nil
}

// selectionSummary formats the concrete normalized selection for the index.
func selectionSummary(selection ResolvedSelection) string {
	limit := "all"
	if selection.Limit > 0 {
		limit = strconv.Itoa(selection.Limit)
	}
	return fmt.Sprintf(
		"state=%s; labels=%s; assignee=%s; author=%s; limit=%s; sort=%s; order=%s",
		selection.State,
		jsonArray(selection.Labels),
		anyOrValue(selection.Assignee),
		anyOrValue(selection.Author),
		limit,
		selection.Sort,
		selection.Order,
	)
}

func anyOrValue(value string) string {
	if value == "" {
		return "any"
	}
	return value
}

// jsonValue renders a string as a JSON-quoted YAML-compatible scalar.
func jsonValue(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// jsonArray renders strings as a compact JSON array, never null.
func jsonArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

// normalizeBody normalizes line endings to LF and removes trailing blank or
// whitespace-only lines while preserving the body's Markdown untouched.
func normalizeBody(body string) string {
	normalized := strings.ReplaceAll(body, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	end := len(lines)
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[:end], "\n")
}

// markdownTableCell makes a value safe inside one Markdown table cell.
func markdownTableCell(value string) string {
	safe := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(value)
	return strings.ReplaceAll(safe, "|", `\|`)
}

// markdownInlineCode wraps a value in a backtick fence longer than any
// consecutive backtick run inside it.
func markdownInlineCode(value string) string {
	longest, current := 0, 0
	for _, character := range value {
		if character == '`' {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 0
		}
	}
	fence := strings.Repeat("`", longest+1)
	padding := ""
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		padding = " "
	}
	return fence + padding + value + padding + fence
}

// labelsCell renders the labels column: an em dash when empty, otherwise
// comma-separated inline code in lexical order.
func labelsCell(labels []string) string {
	if len(labels) == 0 {
		return "—"
	}
	rendered := make([]string, len(labels))
	for i, label := range labels {
		rendered[i] = markdownTableCell(markdownInlineCode(label))
	}
	return strings.Join(rendered, ", ")
}
