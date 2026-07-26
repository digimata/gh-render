# gh-render

`gh render` generates deterministic local Markdown projections of GitHub objects.

The extension is scaffolded but not yet functional. The first supported command will render a repository's issues:

```text
GitHub issues → gh render issues → .issues/*.md
```

GitHub remains authoritative. Rendered files are derived artifacts and are never pushed upstream.

## Installation

During development:

```bash
go build -o gh-render .
gh extension install .
```

After the first release:

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
gh render issues --limit 20
gh render issues --label bug --label p0
gh render issues --state open --assignee @me
```

`gh render` without an object displays help. Only `issues` is in v0 scope.

## Documentation

- [Global specification](docs/spec.md)
- [Issues renderer specification](docs/objects/issues.md)
- [Rendering model decision](docs/.decisions/adr-001-rendering-model.md)
- [v0 implementation plan](.plan/v0.md)
- [Changelog](CHANGELOG.md)

## Development

```bash
go test ./...
go vet ./...
go build -o gh-render .
```

Release packaging lives in `.github/workflows/release.yml`.
