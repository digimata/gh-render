package projection

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// BuildPlan validates the expected files, inventories the output directory,
// and returns a complete plan of creates, updates, and stale removals. It
// never modifies the filesystem. The output argument is the user-supplied
// directory; reported change paths are relative to it.
func BuildPlan(output string, expected []File, ownership Ownership) (Plan, error) {
	if ownership.IsManagedName == nil || ownership.IsManagedContent == nil {
		return Plan{}, errors.New("projection: ownership functions must be set")
	}

	files, err := validateExpected(expected, ownership)
	if err != nil {
		return Plan{}, err
	}

	root, rootExists, err := resolveRoot(output)
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{
		Root:       root,
		Files:      files,
		rootExists: rootExists,
		ownership:  ownership,
	}
	if err := plan.inventory(output); err != nil {
		return Plan{}, err
	}

	sort.Slice(plan.Changes, func(i, j int) bool {
		if plan.Changes[i].Path != plan.Changes[j].Path {
			return plan.Changes[i].Path < plan.Changes[j].Path
		}
		return plan.Changes[i].Kind < plan.Changes[j].Kind
	})
	return plan, nil
}

// validateExpected enforces relative, non-traversing, unique, managed names
// with managed content, and returns the files sorted by name.
func validateExpected(expected []File, ownership Ownership) ([]File, error) {
	files := make([]File, 0, len(expected))
	seen := make(map[string]bool, len(expected))
	for _, file := range expected {
		cleaned := filepath.Clean(file.Name)
		switch {
		case file.Name == "" || cleaned == ".":
			return nil, fmt.Errorf("projection: empty file name")
		case filepath.IsAbs(file.Name):
			return nil, fmt.Errorf("projection: absolute file name %q", file.Name)
		case cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)):
			return nil, fmt.Errorf("projection: traversing file name %q", file.Name)
		case cleaned != file.Name:
			return nil, fmt.Errorf("projection: non-canonical file name %q", file.Name)
		case seen[cleaned]:
			return nil, fmt.Errorf("projection: duplicate file name %q", file.Name)
		case !ownership.IsManagedName(cleaned):
			return nil, fmt.Errorf("projection: unmanaged file name %q", file.Name)
		case !ownership.IsManagedContent(file.Content):
			return nil, fmt.Errorf("projection: file %q is missing the ownership marker", file.Name)
		}
		seen[cleaned] = true
		files = append(files, File{Name: cleaned, Content: file.Content})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

// resolveRoot resolves the output directory to a canonical absolute path by
// canonicalizing its nearest existing ancestor. It reports whether the
// directory itself already exists.
func resolveRoot(output string) (string, bool, error) {
	if output == "" {
		return "", false, errors.New("projection: empty output directory")
	}
	absolute, err := filepath.Abs(output)
	if err != nil {
		return "", false, fmt.Errorf("projection: resolve output directory %q: %w", output, err)
	}

	info, err := os.Lstat(absolute)
	if err == nil {
		if info.Mode()&fs.ModeSymlink != 0 {
			return "", false, fmt.Errorf("projection: output directory %s is a symlink", absolute)
		}
		if !info.IsDir() {
			return "", false, fmt.Errorf("projection: output path %s is a file", absolute)
		}
		canonical, err := filepath.EvalSymlinks(absolute)
		if err != nil {
			return "", false, fmt.Errorf("projection: canonicalize output directory %s: %w", absolute, err)
		}
		return canonical, true, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", false, fmt.Errorf("projection: inspect output directory %s: %w", absolute, err)
	}

	// The directory is missing: canonicalize the nearest existing ancestor
	// and rejoin the missing remainder beneath it.
	remainder := filepath.Base(absolute)
	ancestor := filepath.Dir(absolute)
	for {
		info, err := os.Lstat(ancestor)
		switch {
		case err == nil && info.IsDir():
			canonical, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", false, fmt.Errorf("projection: canonicalize %s: %w", ancestor, err)
			}
			return filepath.Join(canonical, remainder), false, nil
		case err == nil:
			return "", false, fmt.Errorf("projection: output ancestor %s is a file", ancestor)
		case !errors.Is(err, fs.ErrNotExist):
			return "", false, fmt.Errorf("projection: inspect %s: %w", ancestor, err)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", false, fmt.Errorf("projection: no existing ancestor for %s", absolute)
		}
		remainder = filepath.Join(filepath.Base(ancestor), remainder)
		ancestor = parent
	}
}

// inventory compares expected files with disk state, records planned writes
// and removals, and rejects unmanaged or non-regular expected targets.
func (plan *Plan) inventory(output string) error {
	expectedNames := make(map[string]bool, len(plan.Files))
	for _, file := range plan.Files {
		expectedNames[file.Name] = true
		target := filepath.Join(plan.Root, file.Name)
		reported := filepath.Join(output, file.Name)

		info, err := os.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			plan.Changes = append(plan.Changes, Change{Kind: Create, Path: reported})
			plan.writes = append(plan.writes, plannedWrite{target: target, content: file.Content})
			continue
		}
		if err != nil {
			return fmt.Errorf("projection: inspect %s: %w", target, err)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("projection: target %s is a symlink", target)
		}
		if info.IsDir() {
			return fmt.Errorf("projection: target %s is a directory", target)
		}
		existing, err := os.ReadFile(target)
		if err != nil {
			return fmt.Errorf("projection: read %s: %w", target, err)
		}
		if !plan.ownership.IsManagedContent(existing) {
			return fmt.Errorf("projection: refusing to replace unmanaged file %s", target)
		}
		if !bytes.Equal(existing, file.Content) {
			plan.Changes = append(plan.Changes, Change{Kind: Update, Path: reported})
			plan.writes = append(plan.writes, plannedWrite{target: target, content: file.Content})
		}
	}

	if !plan.rootExists {
		return nil
	}
	entries, err := os.ReadDir(plan.Root)
	if err != nil {
		return fmt.Errorf("projection: read output directory %s: %w", plan.Root, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if expectedNames[name] || !plan.ownership.IsManagedName(name) {
			continue
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(plan.Root, name))
		if err != nil {
			return fmt.Errorf("projection: read %s: %w", filepath.Join(plan.Root, name), err)
		}
		if !plan.ownership.IsManagedContent(content) {
			continue
		}
		plan.Changes = append(plan.Changes, Change{Kind: Remove, Path: filepath.Join(output, name)})
		plan.removals = append(plan.removals, plannedRemoval{target: filepath.Join(plan.Root, name)})
	}
	return nil
}

// Apply materializes the plan: it creates the output directory when missing,
// atomically replaces created and updated files, and removes stale managed
// files after revalidating them.
func (plan Plan) Apply() error {
	if err := os.MkdirAll(plan.Root, 0o755); err != nil {
		return fmt.Errorf("projection: create output directory %s: %w", plan.Root, err)
	}
	for _, write := range plan.writes {
		if err := replaceFile(write.target, write.content); err != nil {
			return err
		}
	}
	for _, removal := range plan.removals {
		if err := plan.removeStale(removal.target); err != nil {
			return err
		}
	}
	return nil
}

// removeStale revalidates a planned removal immediately before deletion and
// aborts if its type or ownership changed since planning.
func (plan Plan) removeStale(target string) error {
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("projection: inspect %s: %w", target, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("projection: refusing to remove non-regular file %s", target)
	}
	content, err := os.ReadFile(target)
	if err != nil {
		return fmt.Errorf("projection: read %s: %w", target, err)
	}
	if !plan.ownership.IsManagedContent(content) {
		return fmt.Errorf("projection: refusing to remove unmanaged file %s", target)
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("projection: remove %s: %w", target, err)
	}
	return nil
}

// replaceFile writes content through a same-directory temporary file and an
// atomic rename, removing the temporary file on every failure path.
func replaceFile(target string, content []byte) (err error) {
	temporary, err := os.CreateTemp(filepath.Dir(target), ".gh-render-*")
	if err != nil {
		return fmt.Errorf("projection: create temporary file for %s: %w", target, err)
	}
	defer func() {
		if err != nil {
			temporary.Close()
			os.Remove(temporary.Name())
		}
	}()

	if _, err = temporary.Write(content); err != nil {
		return fmt.Errorf("projection: write %s: %w", target, err)
	}
	if err = temporary.Chmod(0o644); err != nil {
		return fmt.Errorf("projection: set mode on %s: %w", target, err)
	}
	if err = temporary.Sync(); err != nil {
		return fmt.Errorf("projection: sync %s: %w", target, err)
	}
	if err = temporary.Close(); err != nil {
		return fmt.Errorf("projection: close temporary file for %s: %w", target, err)
	}
	if err = os.Rename(temporary.Name(), target); err != nil {
		os.Remove(temporary.Name())
		return fmt.Errorf("projection: replace %s: %w", target, err)
	}
	return nil
}
