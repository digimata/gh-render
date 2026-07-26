# gh-render

> A GitHub CLI extension that renders GitHub objects into deterministic local Markdown projections. GitHub remains authoritative; local files become searchable, linkable repository artifacts.

The issues renderer is implemented and verified against a live repository. `v0.1.0` is the first release target.

## Table of Contents

1. [Why gh-render exists](#1-why-gh-render-exists)
2. [Installation](#2-installation)
3. [Usage](#3-usage)
4. [Selecting issues](#4-selecting-issues)
5. [Generated projection](#5-generated-projection)
6. [Safety model](#6-safety-model)
7. [Repository map](#7-repository-map)
8. [Development](#8-development)

## 1. Why gh-render exists

GitHub issues are part of a repository's working context, but they normally remain outside its filesystem. This limits ordinary search, local links, offline inspection, and agent access.

`gh render` materializes a read-only local view:

```text
GitHub issues → gh render issues → .issues/*.md
```

The projection can be committed and reviewed without becoming a second issue tracker. Rendering never writes to GitHub.

## 2. Installation

Once `v0.1.0` is released:

```bash
gh extension install digimata/gh-render
```

To build and install the current source:

```bash
git clone https://github.com/digimata/gh-render.git
cd gh-render
go build -o gh-render .
gh extension install .
```

The extension is invoked as `gh render`.

## 3. Usage

Render the current repository's issues:

```bash
gh render issues
```

Render an explicit repository or output directory:

```bash
gh render issues --repo owner/repository
gh render issues --output .issues
```

Inspect changes without writing, or verify that a committed projection is current:

```bash
gh render issues --dry-run
gh render issues --check
```

## 4. Selecting issues

With no selectors, `gh render issues` includes every open and closed issue.

Render the 20 most recently updated issues:

```bash
gh render issues --limit 20
```

Filter by state, labels, assignee, or author:

```bash
gh render issues --state open
gh render issues --label bug --label p0
gh render issues --assignee @me
gh render issues --author @me
```

Different selector types combine with AND. Repeated labels require every label. `@me` resolves to the authenticated GitHub login.

Control ranking before a limit is applied:

```bash
gh render issues --limit 10 --sort created --order desc
```

The [issues renderer specification](docs/objects/issues.md) defines the complete selection and tie-breaking contract.

## 5. Generated projection

The default output is:

```text
.issues/
├── index.md
├── iss-0001.md
├── iss-0002.md
└── iss-0123.md
```

Each issue file contains GitHub metadata, a source link, and the issue body. The index records the source repository and normalized selection.

Generated files contain an ownership marker. A second render against unchanged GitHub data produces byte-identical output.

## 6. Safety model

`gh-render` follows four filesystem rules:

1. It replaces or removes only files carrying its ownership marker.
2. It refuses the entire operation when a target file is unmanaged.
3. It validates the complete write plan before changing disk state.
4. It writes each file through an atomic replacement.

A filename matching the active renderer's managed pattern is reserved. If an unmanaged regular file occupies such a name—even outside the current selection—the complete render fails before mutation. Unrelated files, directories, and symlinks are ignored.

A filtered render is a complete projection of that filter. Managed files outside the selected set become stale and are removed in normal mode. Use `--dry-run` before changing an existing projection.

The [global specification](docs/spec.md) defines deterministic output, path validation, exit codes, and validation modes.

## 7. Repository map

```text
gh-render/
├── main.go                  # process entry point
├── internal/
│   ├── app/                 # object dispatch, flags, orchestration, exit codes
│   ├── issues/              # issue fetch, normalization, selection, rendering
│   └── projection/          # write planning and filesystem safety
├── tests/                   # black-box test suites and golden fixtures
├── docs/
│   ├── spec.md              # cross-renderer behavioral contract
│   ├── objects/             # object-specific specifications
│   └── .decisions/          # architecture decisions
├── .plan/                   # approved implementation plans
├── .github/workflows/       # precompiled extension releases
└── CHANGELOG.md
```

The current implementation plan is [.plan/v0.md](.plan/v0.md). The detailed coding handoff is [.plan/v0-implementation.md](.plan/v0-implementation.md). The rendering-model decision is [ADR-001](docs/.decisions/adr-001-rendering-model.md).

## 8. Development

Requirements:

- Go version declared in `go.mod`;
- an authenticated GitHub CLI session.

Validation:

```bash
go test ./...
go test -race ./...
go vet ./...
go build -o gh-render .
```

Automated tests never use the network or the local GitHub configuration. All `_test.go` files and golden fixtures live under `tests/`; production packages contain no test files.

Start with the [global specification](docs/spec.md), then read the specification for the object being changed. Update `CHANGELOG.md` with user-visible behavior.
