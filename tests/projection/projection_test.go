// Package projection_test black-box tests projection planning and writing
// against temporary directories.
package projection_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/digimata/gh-render/internal/projection"
)

// testOwnership treats index.md and iss-*.md as managed names and any content
// starting with the managed marker as managed content.
func testOwnership() projection.Ownership {
	return projection.Ownership{
		IsManagedName: func(name string) bool {
			return name == "index.md" ||
				(strings.HasPrefix(name, "iss-") && strings.HasSuffix(name, ".md"))
		},
		IsManagedContent: func(content []byte) bool {
			return bytes.HasPrefix(content, []byte(projection.ManagedMarker))
		},
	}
}

// managed returns marker-prefixed managed file content.
func managed(body string) []byte {
	return []byte(projection.ManagedMarker + "\n\n" + body + "\n")
}

// writeFile writes content, failing the test on error.
func writeFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// buildPlan builds a plan with the test ownership, failing the test on error.
func buildPlan(t *testing.T, output string, expected []projection.File) projection.Plan {
	t.Helper()
	plan, err := projection.BuildPlan(output, expected, testOwnership())
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	return plan
}

func changeStrings(changes []projection.Change) []string {
	rendered := make([]string, len(changes))
	for i, change := range changes {
		rendered[i] = change.Kind.String() + " " + change.Path
	}
	return rendered
}

func TestChangeKindString(t *testing.T) {
	tests := []struct {
		kind     projection.ChangeKind
		expected string
	}{
		{projection.Create, "create"},
		{projection.Update, "update"},
		{projection.Remove, "remove"},
		{projection.ChangeKind(200), "unknown"},
	}
	for _, test := range tests {
		if got := test.kind.String(); got != test.expected {
			t.Errorf("ChangeKind(%d).String() = %q, want %q", test.kind, got, test.expected)
		}
	}
}

func TestBuildPlanRequiresOwnershipFunctions(t *testing.T) {
	_, err := projection.BuildPlan(t.TempDir(), nil, projection.Ownership{})
	if err == nil || !strings.Contains(err.Error(), "ownership functions") {
		t.Errorf("error = %v, want ownership requirement", err)
	}
}

func TestBuildPlanRejectsInvalidExpectedNames(t *testing.T) {
	tests := []struct {
		name     string
		files    []projection.File
		fragment string
	}{
		{"empty name", []projection.File{
			{Name: "", Content: managed("a")},
		}, "empty file name"},
		{"absolute name", []projection.File{
			{Name: "/etc/iss-0001.md", Content: managed("a")},
		}, "absolute file name"},
		{"traversing name", []projection.File{
			{Name: "../iss-0001.md", Content: managed("a")},
		}, "traversing file name"},
		{"duplicate name", []projection.File{
			{Name: "iss-0001.md", Content: managed("a")},
			{Name: "iss-0001.md", Content: managed("b")},
		}, "duplicate file name"},
		{"unmanaged name", []projection.File{
			{Name: "notes.md", Content: managed("a")},
		}, "unmanaged file name"},
		{"unmanaged content", []projection.File{
			{Name: "iss-0001.md", Content: []byte("no marker\n")},
		}, "missing the ownership marker"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			_, err := projection.BuildPlan(root, test.files, testOwnership())
			if err == nil || !strings.Contains(err.Error(), test.fragment) {
				t.Errorf("error = %v, want %q", err, test.fragment)
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 0 {
				t.Errorf("output directory touched: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestBuildPlanRejectsBadOutputPath(t *testing.T) {
	base := t.TempDir()

	filePath := filepath.Join(base, "occupied")
	writeFile(t, filePath, []byte("not a directory\n"))
	if _, err := projection.BuildPlan(filePath, nil, testOwnership()); err == nil ||
		!strings.Contains(err.Error(), "is a file") {
		t.Errorf("file output error = %v, want rejection", err)
	}

	realDirectory := filepath.Join(base, "real")
	if err := os.Mkdir(realDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(base, "link")
	if err := os.Symlink(realDirectory, linkPath); err != nil {
		t.Fatal(err)
	}
	if _, err := projection.BuildPlan(linkPath, nil, testOwnership()); err == nil ||
		!strings.Contains(err.Error(), "is a symlink") {
		t.Errorf("symlink output error = %v, want rejection", err)
	}
}

func TestBuildPlanMissingOutputDirectory(t *testing.T) {
	output := filepath.Join(t.TempDir(), "projection")
	expected := []projection.File{
		{Name: "index.md", Content: managed("index")},
		{Name: "iss-0001.md", Content: managed("one")},
	}

	plan := buildPlan(t, output, expected)
	want := []string{
		"create " + filepath.Join(output, "index.md"),
		"create " + filepath.Join(output, "iss-0001.md"),
	}
	if got := changeStrings(plan.Changes); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("changes = %v, want %v", got, want)
	}
	// Planning alone (check and dry-run) must not create the directory.
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("BuildPlan created the output directory: %v", err)
	}
}

func TestBuildPlanClassifiesChanges(t *testing.T) {
	output := t.TempDir()
	writeFile(t, filepath.Join(output, "iss-0001.md"), managed("unchanged"))
	writeFile(t, filepath.Join(output, "iss-0002.md"), managed("old"))
	writeFile(t, filepath.Join(output, "iss-0009.md"), managed("stale"))
	expected := []projection.File{
		{Name: "index.md", Content: managed("index")},        // missing → create
		{Name: "iss-0001.md", Content: managed("unchanged")}, // equal bytes → no-op
		{Name: "iss-0002.md", Content: managed("new")},       // differing bytes → update
	}

	plan := buildPlan(t, output, expected)
	want := []string{
		"create " + filepath.Join(output, "index.md"),
		"update " + filepath.Join(output, "iss-0002.md"),
		"remove " + filepath.Join(output, "iss-0009.md"),
	}
	if got := changeStrings(plan.Changes); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("changes = %v, want %v", got, want)
	}
	if plan.IsCurrent() {
		t.Error("plan with changes reports current")
	}
}

func TestBuildPlanChangesSortByPath(t *testing.T) {
	output := t.TempDir()
	writeFile(t, filepath.Join(output, "iss-0001.md"), managed("stale"))
	writeFile(t, filepath.Join(output, "iss-0003.md"), managed("old"))
	expected := []projection.File{
		{Name: "iss-0004.md", Content: managed("d")},
		{Name: "index.md", Content: managed("index")},
		{Name: "iss-0003.md", Content: managed("new")},
	}

	plan := buildPlan(t, output, expected)
	want := []string{
		"create " + filepath.Join(output, "index.md"),
		"remove " + filepath.Join(output, "iss-0001.md"),
		"update " + filepath.Join(output, "iss-0003.md"),
		"create " + filepath.Join(output, "iss-0004.md"),
	}
	if got := changeStrings(plan.Changes); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("changes = %v, want %v", got, want)
	}
}

func TestBuildPlanRejectsUnmanagedExpectedTarget(t *testing.T) {
	output := t.TempDir()
	original := []byte("# Hand-written notes\n")
	writeFile(t, filepath.Join(output, "iss-0001.md"), original)

	_, err := projection.BuildPlan(output, []projection.File{
		{Name: "iss-0001.md", Content: managed("generated")},
	}, testOwnership())
	if err == nil || !strings.Contains(err.Error(), "unmanaged file") {
		t.Fatalf("error = %v, want unmanaged-target rejection", err)
	}
	surviving, readErr := os.ReadFile(filepath.Join(output, "iss-0001.md"))
	if readErr != nil || !bytes.Equal(surviving, original) {
		t.Errorf("unmanaged file modified: %q err=%v", surviving, readErr)
	}
}

func TestBuildPlanRejectsDirectoryAndSymlinkTargets(t *testing.T) {
	t.Run("directory", func(t *testing.T) {
		output := t.TempDir()
		if err := os.Mkdir(filepath.Join(output, "iss-0001.md"), 0o755); err != nil {
			t.Fatal(err)
		}
		_, err := projection.BuildPlan(output, []projection.File{
			{Name: "iss-0001.md", Content: managed("a")},
		}, testOwnership())
		if err == nil || !strings.Contains(err.Error(), "is a directory") {
			t.Errorf("error = %v, want directory rejection", err)
		}
	})
	t.Run("symlink", func(t *testing.T) {
		output := t.TempDir()
		pointee := filepath.Join(output, "elsewhere.txt")
		writeFile(t, pointee, managed("elsewhere"))
		if err := os.Symlink(pointee, filepath.Join(output, "iss-0001.md")); err != nil {
			t.Fatal(err)
		}
		_, err := projection.BuildPlan(output, []projection.File{
			{Name: "iss-0001.md", Content: managed("a")},
		}, testOwnership())
		if err == nil || !strings.Contains(err.Error(), "is a symlink") {
			t.Errorf("error = %v, want symlink rejection", err)
		}
	})
}

func TestBuildPlanIgnoresUnownedStrays(t *testing.T) {
	output := t.TempDir()
	unmanagedContent := []byte("# my own iss file\n")
	writeFile(t, filepath.Join(output, "iss-7777.md"), unmanagedContent)          // managed name, unmanaged content
	writeFile(t, filepath.Join(output, "notes.md"), []byte("notes\n"))            // unmanaged name
	if err := os.Mkdir(filepath.Join(output, "iss-6666.md"), 0o755); err != nil { // directory with managed name
		t.Fatal(err)
	}
	pointee := filepath.Join(output, "pointee.txt")
	writeFile(t, pointee, managed("looks managed"))
	if err := os.Symlink(pointee, filepath.Join(output, "iss-5555.md")); err != nil { // symlink with managed name
		t.Fatal(err)
	}

	plan := buildPlan(t, output, []projection.File{
		{Name: "index.md", Content: managed("index")},
	})
	for _, change := range plan.Changes {
		if change.Kind == projection.Remove {
			t.Errorf("unexpected removal planned: %v", change)
		}
	}
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	surviving, err := os.ReadFile(filepath.Join(output, "iss-7777.md"))
	if err != nil || !bytes.Equal(surviving, unmanagedContent) {
		t.Errorf("unmanaged stray modified: %q err=%v", surviving, err)
	}
	if _, err := os.Lstat(filepath.Join(output, "iss-5555.md")); err != nil {
		t.Errorf("symlink removed: %v", err)
	}
	if content, err := os.ReadFile(pointee); err != nil || !bytes.Equal(content, managed("looks managed")) {
		t.Errorf("symlink was followed; pointee = %q err=%v", content, err)
	}
	if info, err := os.Stat(filepath.Join(output, "iss-6666.md")); err != nil || !info.IsDir() {
		t.Errorf("directory stray removed: %v", err)
	}
}
