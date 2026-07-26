# gh-render

`gh render` generates deterministic local Markdown projections of GitHub objects.

The extension is scaffolded but not yet functional. The first renderer will materialize a repository's issues under `.issues/`:

```text
GitHub issues → gh render issues → .issues/*.md
```

GitHub remains authoritative. Rendered files are derived artifacts and are never pushed back upstream.

## Installation

During local development:

```bash
go build -o gh-render .
gh extension install .
```

After publication:

```bash
gh extension install digimata/gh-render
```

## Planned interface

```bash
gh render issues
gh render issues --repo owner/repo
gh render issues --output .issues
gh render issues --check
gh render issues --dry-run
```

`gh render` without an object displays help. Only the `issues` object is planned for the initial release.

## Issues renderer contract

The initial renderer will:

1. Infer the repository from the current directory unless `--repo` is provided.
2. Fetch open and closed issues.
3. Write `.issues/iss-NNNN.md` and `.issues/index.md`.
4. Preserve issue bodies without substantive rewriting.
5. Produce deterministic output.
6. Write files atomically.
7. Remove stale files only when they carry the extension's generated marker.
8. Refuse to overwrite unmanaged files.
9. Never write to GitHub.

## Development

```bash
go test ./...
go vet ./...
go build -o gh-render .
```

The repository was created with GitHub CLI's precompiled Go extension scaffold. Release packaging lives in `.github/workflows/release.yml`.
