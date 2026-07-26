package projection_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/digimata/gh-render/internal/issues"
	"github.com/digimata/gh-render/internal/projection"
)

// assertNoTemporaryFiles fails when Apply left .gh-render-* files behind.
func assertNoTemporaryFiles(t *testing.T, root string) {
	t.Helper()
	leftovers, err := filepath.Glob(filepath.Join(root, ".gh-render-*"))
	if err != nil {
		t.Fatalf("glob temporary files: %v", err)
	}
	if len(leftovers) != 0 {
		t.Errorf("temporary files left behind: %v", leftovers)
	}
}

func TestApplyCreatesDirectoryAndFiles(t *testing.T) {
	output := filepath.Join(t.TempDir(), "projection")
	expected := []projection.File{
		{Name: "index.md", Content: managed("index")},
		{Name: "iss-0001.md", Content: managed("one")},
	}

	plan := buildPlan(t, output, expected)
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for _, file := range expected {
		path := filepath.Join(output, file.Name)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if !bytes.Equal(content, file.Content) {
			t.Errorf("%s = %q, want %q", file.Name, content, file.Content)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if mode := info.Mode().Perm(); mode != fs.FileMode(0o644) {
			t.Errorf("%s mode = %v, want 0644", file.Name, mode)
		}
	}
	assertNoTemporaryFiles(t, output)
}

func TestApplyUpdatesAndRemoves(t *testing.T) {
	output := t.TempDir()
	writeFile(t, filepath.Join(output, "iss-0002.md"), managed("old"))
	writeFile(t, filepath.Join(output, "iss-0009.md"), managed("stale"))
	expected := []projection.File{
		{Name: "iss-0002.md", Content: managed("new")},
	}

	plan := buildPlan(t, output, expected)
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(output, "iss-0002.md"))
	if err != nil || !bytes.Equal(content, managed("new")) {
		t.Errorf("updated file = %q err=%v", content, err)
	}
	if _, err := os.Lstat(filepath.Join(output, "iss-0009.md")); !os.IsNotExist(err) {
		t.Errorf("stale managed file survived: %v", err)
	}
	assertNoTemporaryFiles(t, output)
}

func TestSecondPlanAfterApplyIsCurrent(t *testing.T) {
	output := filepath.Join(t.TempDir(), "projection")
	expected := []projection.File{
		{Name: "index.md", Content: managed("index")},
		{Name: "iss-0001.md", Content: managed("one")},
	}

	if err := buildPlan(t, output, expected).Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	second := buildPlan(t, output, expected)
	if !second.IsCurrent() {
		t.Errorf("second plan not current: %v", changeStrings(second.Changes))
	}
	if len(second.Changes) != 0 {
		t.Errorf("second plan changes = %v, want none", second.Changes)
	}
}

func TestApplyWriteFailureCleansTemporaryFiles(t *testing.T) {
	output := t.TempDir()
	plan := buildPlan(t, output, []projection.File{
		{Name: "iss-0001.md", Content: managed("one")},
	})

	// Occupy the planned create target with a directory so the rename fails.
	if err := os.Mkdir(filepath.Join(output, "iss-0001.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := plan.Apply()
	if err == nil {
		t.Fatal("Apply succeeded, want rename failure")
	}
	if info, statErr := os.Stat(filepath.Join(output, "iss-0001.md")); statErr != nil || !info.IsDir() {
		t.Errorf("occupying directory disturbed: %v", statErr)
	}
	assertNoTemporaryFiles(t, output)
}

func TestApplyRevalidatesRemovals(t *testing.T) {
	t.Run("rewritten as unmanaged", func(t *testing.T) {
		output := t.TempDir()
		stale := filepath.Join(output, "iss-0009.md")
		writeFile(t, stale, managed("stale"))
		plan := buildPlan(t, output, []projection.File{
			{Name: "index.md", Content: managed("index")},
		})

		// The user reclaims the file between planning and application.
		reclaimed := []byte("# mine now\n")
		writeFile(t, stale, reclaimed)
		err := plan.Apply()
		if err == nil || !strings.Contains(err.Error(), "refusing to remove unmanaged file") {
			t.Fatalf("Apply error = %v, want unmanaged-removal refusal", err)
		}
		surviving, readErr := os.ReadFile(stale)
		if readErr != nil || !bytes.Equal(surviving, reclaimed) {
			t.Errorf("reclaimed file lost: %q err=%v", surviving, readErr)
		}
	})
	t.Run("replaced by directory", func(t *testing.T) {
		output := t.TempDir()
		stale := filepath.Join(output, "iss-0009.md")
		writeFile(t, stale, managed("stale"))
		plan := buildPlan(t, output, []projection.File{
			{Name: "index.md", Content: managed("index")},
		})

		if err := os.Remove(stale); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(stale, 0o755); err != nil {
			t.Fatal(err)
		}
		err := plan.Apply()
		if err == nil || !strings.Contains(err.Error(), "non-regular file") {
			t.Fatalf("Apply error = %v, want non-regular refusal", err)
		}
		if info, statErr := os.Stat(stale); statErr != nil || !info.IsDir() {
			t.Errorf("replacement directory disturbed: %v", statErr)
		}
	})
}

func TestApplyPreservesEmbeddedMarkerTextInUnmanagedFile(t *testing.T) {
	output := t.TempDir()
	unmanaged := filepath.Join(output, "iss-9999.md")
	content := []byte("This note mentions <!-- gh-render:managed --> in prose.\n\n# Personal notes\n")
	if err := os.WriteFile(unmanaged, content, 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := projection.BuildPlan(
		output,
		[]projection.File{{
			Name:    "index.md",
			Content: []byte(projection.ManagedMarker + "\n\n# Issues\n"),
		}},
		projection.Ownership{
			IsManagedName:    issues.IsManagedName,
			IsManagedContent: issues.IsManagedContent,
		},
	)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	surviving, err := os.ReadFile(unmanaged)
	if err != nil {
		t.Fatalf("read unmanaged file: %v", err)
	}
	if !bytes.Equal(surviving, content) {
		t.Errorf("unmanaged file changed: %q", surviving)
	}
}
