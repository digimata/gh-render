# Changelog

All notable changes to `gh-render` are recorded here.

## 0.1.2 — 2026.07.26

### Added

- A deterministic `Data as of` index line recording the newest `updated_at` across the selected issues.

### Changed

- The issue file's GitHub link now reads `#7 — Title` instead of the bare number.
- Index table entries now read `ISS-0007 — Title`, matching the issue heading form.

### Fixed

- Reject unmanaged regular files that occupy renderer-owned names before any projection mutation.

## 0.1.1 — 2026.07.26

### Fixed

- Require the ownership marker to occupy its own line so embedded marker text cannot grant file ownership.

## 0.1.0 — 2026.07.26

### Added

- Initial `gh render <object>` command shell.
- Approved global rendering and issues-object specifications.
- Issue selection contract for state, labels, assignee, author, ranking, and limits.
- Rendering-model architecture decision.
- v0 implementation and release plan.
- Contributor-oriented README and simplified two-package v0 source layout.
- Detailed v0 coding handoff with concrete types, package APIs, algorithms, and tests.
- Top-level black-box test layout that keeps production packages free of test files.
- `gh render issues`: complete paginated retrieval, pull-request exclusion, and a normalized issue model.
- State, label, assignee, and author selectors with AND semantics, `@me` resolution, deterministic ranking, and `--limit`.
- Deterministic issue and index Markdown with JSON-quoted frontmatter and a recorded normalized selection.
- Managed-file ownership protection, atomic per-file replacement, and safe stale-file removal.
- `--dry-run` change reporting and `--check` staleness validation with the documented exit codes.
