// Package projection plans and applies deterministic local file projections.
// It owns generic write planning and filesystem safety; object renderers
// produce the expected files it materializes.
package projection

// ManagedMarker is the exact ownership marker every managed file contains.
const ManagedMarker = "<!-- gh-render:managed -->"

// File is one expected projection file. Name is relative to the output root.
type File struct {
	Name    string
	Content []byte
}

// Ownership decides which names and contents the active renderer owns.
type Ownership struct {
	IsManagedName    func(name string) bool
	IsManagedContent func(content []byte) bool
}

// ChangeKind classifies one planned filesystem change.
type ChangeKind uint8

const (
	Create ChangeKind = iota
	Update
	Remove
)

// String returns the lowercase action verb for reporting.
func (kind ChangeKind) String() string {
	switch kind {
	case Create:
		return "create"
	case Update:
		return "update"
	case Remove:
		return "remove"
	}
	return "unknown"
}

// Change is one planned filesystem change. Path is the user-facing path
// relative to the invocation, suitable for reporting.
type Change struct {
	Kind ChangeKind
	Path string
}

// Plan is a validated write plan. A Plan is only produced by BuildPlan after
// every expected file and existing target passed the safety checks.
type Plan struct {
	Root    string
	Files   []File
	Changes []Change

	rootExists bool
	writes     []plannedWrite
	removals   []plannedRemoval
	ownership  Ownership
}

// plannedWrite is one create or update against an absolute target path.
type plannedWrite struct {
	target  string
	content []byte
}

// plannedRemoval is one stale managed file scheduled for deletion.
type plannedRemoval struct {
	target string
}

// IsCurrent reports whether the projection on disk already matches the plan.
func (plan Plan) IsCurrent() bool {
	return len(plan.Changes) == 0
}
