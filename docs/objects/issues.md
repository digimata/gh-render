---
title: "Issues renderer specification"
status: approved
version: "0.1"
date: 2026-07-26
object: issues
---

# Issues Renderer Specification

> `gh render issues` renders every issue in a GitHub repository into a deterministic `.issues/` Markdown projection. Pull requests and comments are excluded from v0.

## Table of Contents

1. [Command](#1-command)
2. [Source selection](#2-source-selection)
3. [Normalized issue model](#3-normalized-issue-model)
4. [Output layout](#4-output-layout)
5. [Issue file](#5-issue-file)
6. [Index file](#6-index-file)
7. [Stale-file behavior](#7-stale-file-behavior)
8. [Acceptance criteria](#8-acceptance-criteria)

## 1. Command

```text
gh render issues [flags]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `--repo owner/repo` | Current repository | Repository whose issues are rendered. |
| `--output <directory>` | `.issues` | Projection directory, resolved from the current working directory. |
| `--check` | `false` | Fail without writing when the projection is stale. |
| `--dry-run` | `false` | Report intended changes without writing. |

Open and closed issues are always included in v0. State filtering is deferred because a complete local projection is the default use case.

## 2. Source selection

The renderer fetches the complete issues collection with pagination. It includes ordinary GitHub issues in open and closed states and excludes pull requests returned through shared issue APIs.

The v0 projection excludes:

- issue comments;
- reactions;
- project fields;
- timeline events;
- linked pull-request details;
- deleted issues unavailable through the authenticated API.

Records are sorted by ascending issue number after normalization. API response order is never trusted.

## 3. Normalized issue model

The GitHub client converts API responses into this internal model before rendering:

```text
Issue
├── number
├── title
├── body
├── state
├── url
├── labels[]
├── assignees[]
├── milestone?
├── createdAt
└── updatedAt
```

Labels and assignees are sorted lexically by their rendered names. Missing bodies become an empty string. Missing labels and assignees become empty arrays. A missing milestone becomes null.

Timestamps are preserved as UTC RFC 3339 strings. Issue state is rendered as lowercase `open` or `closed`.

## 4. Output layout

The default projection is:

```text
.issues/
├── index.md
├── iss-0001.md
├── iss-0002.md
└── iss-0123.md
```

Issue filenames use `iss-` followed by the issue number padded to at least four digits. Numbers longer than four digits are not truncated.

The output directory contains a flat collection. The renderer does not create state or label subdirectories.

## 5. Issue file

Each issue file begins with YAML-compatible frontmatter:

```yaml
---
id: 7
title: "Make file imports atomically non-overwriting"
status: open
source: github
github_url: "https://github.com/owner/repository/issues/7"
labels: []
assignees: []
milestone: null
created_at: "2026-07-26T19:37:25Z"
updated_at: "2026-07-26T19:37:25Z"
---
```

String and array values use JSON-compatible quoting inside the YAML document. This avoids ambiguous escaping while remaining valid YAML.

The ownership marker follows the frontmatter:

```html
<!-- gh-render:managed -->
```

The visible document begins:

```markdown
# ISS-0007 — Make file imports atomically non-overwriting

GitHub: [#7](https://github.com/owner/repository/issues/7)
```

The GitHub issue body follows the link. The renderer does not rewrite headings, lists, links, or prose. It normalizes line endings to LF, removes trailing blank lines, and writes one final newline.

## 6. Index file

`.issues/index.md` begins with the ownership marker and identifies the source repository:

```markdown
<!-- gh-render:managed -->

# Issues

> Generated from [owner/repository](https://github.com/owner/repository/issues). GitHub is canonical. Run `gh render issues` to refresh this projection.
```

The issue table is sorted by ascending issue number:

```markdown
| Issue | State | Labels | Updated |
| --- | --- | --- | --- |
| [0007 — Make file imports atomically non-overwriting](iss-0007.md) | open | — | 2026-07-26 |
```

Pipes and newlines in table cells are escaped. Empty labels render as an em dash. Nonempty labels render as comma-separated inline-code values in lexical order.

## 7. Stale-file behavior

The issues renderer owns:

- `.issues/index.md` when it contains the ownership marker; and
- direct children matching `iss-\d{4,}\.md` when they contain the marker.

If an upstream issue disappears, the corresponding managed issue file becomes stale and is removed in normal mode. Unmanaged matching files cause the render to fail before any write or deletion.

Other files and subdirectories inside `.issues/` are ignored. Cleanup does not follow symlinks.

## 8. Acceptance criteria

The v0 issues renderer is complete when:

1. It renders all seven Terrazzo issues with the expected metadata and bodies.
2. Pull requests are absent.
3. The first render creates the expected index and issue files.
4. A second unchanged render produces no diff.
5. `--dry-run` reports changes without touching file content or timestamps.
6. `--check` returns `0` when current and `3` when stale.
7. An unmanaged target prevents every write.
8. A removed upstream issue deletes only its managed snapshot.
9. Titles, labels, assignees, milestones, empty bodies, Markdown bodies, and Unicode round-trip safely.
10. Terrazzo can remove its repository-local sync script without changing the intended projection.
