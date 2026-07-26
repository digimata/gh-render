---
title: "Issues renderer specification"
status: approved
version: "0.1"
date: 2026-07-26
object: issues
---

# Issues Renderer Specification

> `gh render issues` renders a complete selected set of GitHub issues into a deterministic `.issues/` Markdown projection. Pull requests and comments are excluded from v0.

## Table of Contents

1. [Command](#1-command)
2. [Source selection](#2-source-selection)
3. [Filtering, ranking, and limits](#3-filtering-ranking-and-limits)
4. [Normalized issue model](#4-normalized-issue-model)
5. [Output layout](#5-output-layout)
6. [Issue file](#6-issue-file)
7. [Index file](#7-index-file)
8. [Stale-file behavior](#8-stale-file-behavior)
9. [Acceptance criteria](#9-acceptance-criteria)

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
| `--state open\|closed\|all` | `all` | Include issues in the selected state. |
| `--label <name>` | None | Require a label. May be repeated. |
| `--assignee <login\|@me>` | Any | Require the named assignee. |
| `--author <login\|@me>` | Any | Require the named issue author. |
| `--limit <number>` | Unlimited | Keep the highest-ranked positive number of matches. |
| `--sort updated\|created\|number` | `updated` | Field used to rank matches before limiting. |
| `--order asc\|desc` | `desc` | Ranking direction. |

With no selectors or limit, the renderer includes every open and closed issue. `--check` and `--dry-run` remain mutually exclusive.

## 2. Source selection

The renderer fetches the complete issues collection with pagination. It includes ordinary GitHub issues in open and closed states and excludes pull requests returned through shared issue APIs.

The v0 projection excludes:

- issue comments;
- reactions;
- project fields;
- timeline events;
- linked pull-request details;
- deleted issues unavailable through the authenticated API.

API response order is never trusted. The renderer retrieves enough matching data to apply selection and limits correctly.

## 3. Filtering, ranking, and limits

Selection follows this sequence:

1. Resolve `@me` to the authenticated GitHub login.
2. Apply the state selector.
3. Require every repeated `--label` value.
4. Apply the assignee selector.
5. Apply the author selector.
6. Rank every remaining match.
7. Apply the limit when present.
8. Sort the selected set by ascending issue number for serialization.

Different selector types combine with AND. Repeated labels also use AND semantics: `--label bug --label p0` selects issues containing both labels. Label names and GitHub logins use GitHub's case-insensitive matching semantics.

`--assignee @me` selects issues assigned to the authenticated user. `--author @me` selects issues opened by that user. Before rendering, both aliases resolve to a concrete login so the projection does not contain `@me`.

`--limit` accepts a positive integer. Zero, negative, missing, or nonnumeric values are usage errors.

Ranking uses `--sort` and `--order`. The default ranking is most recently updated first. An issue-number comparison in the selected order breaks equal-field ties. Ranking determines membership only; the rendered index and files remain ordered by ascending issue number.

Examples:

```bash
gh render issues --limit 20
gh render issues --state open --label bug --label p0
gh render issues --state open --assignee @me --limit 10
gh render issues --author @me --sort created --order desc
```

A filtered render is the complete projection of its normalized selection. Managed issue files outside the selected set become stale; they are reported by `--dry-run`, rejected by stale `--check`, and removed in normal mode.

## 4. Normalized issue model

The GitHub client converts API responses into this internal model before rendering:

```text
Issue
├── number
├── title
├── body
├── state
├── author
├── url
├── labels[]
├── assignees[]
├── milestone?
├── createdAt
└── updatedAt
```

Labels and assignees are sorted lexically by their rendered names. Missing bodies become an empty string. Missing labels and assignees become empty arrays. A missing milestone becomes null.

Timestamps are preserved as UTC RFC 3339 strings. Issue state is rendered as lowercase `open` or `closed`.

## 5. Output layout

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

## 6. Issue file

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

## 7. Index file

`.issues/index.md` begins with the ownership marker and identifies the source repository:

```markdown
<!-- gh-render:managed -->

# Issues

> Generated from [owner/repository](https://github.com/owner/repository/issues). GitHub is canonical.
>
> Selection: state=open; labels=["bug","p0"]; assignee=dremnik; author=any; limit=20; sort=updated; order=desc.
```

The selection line always appears and uses concrete normalized values. Unset selectors render as `any`; an omitted limit renders as `all`. Labels use a compact JSON array sorted lexically. The output path is not embedded because it may contain a host-specific absolute path.

The issue table is sorted by ascending issue number:

```markdown
| Issue | State | Labels | Updated |
| --- | --- | --- | --- |
| [0007 — Make file imports atomically non-overwriting](iss-0007.md) | open | — | 2026-07-26 |
```

Pipes and newlines in table cells are escaped. Empty labels render as an em dash. Nonempty labels render as comma-separated inline-code values in lexical order.

## 8. Stale-file behavior

The issues renderer owns:

- `.issues/index.md` when it contains the ownership marker; and
- direct children matching `iss-\d{4,}\.md` when they contain the marker.

If an upstream issue disappears or no longer belongs to the active selection, the corresponding managed issue file becomes stale and is removed in normal mode. Changing from a full projection to a filtered projection may therefore remove managed snapshots. `--dry-run` reports those removals before migration.

Unmanaged matching files cause the render to fail before any write or deletion.

Other files and subdirectories inside `.issues/` are ignored. Cleanup does not follow symlinks.

## 9. Acceptance criteria

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
10. `--limit 3` selects the three highest-ranked matches and serializes them by issue number.
11. State, label, assignee, and author selectors combine with AND.
12. Repeated labels require every named label.
13. `@me` resolves to the authenticated login and the concrete login appears in the index.
14. Equal ranking fields use issue number as a deterministic tie-breaker.
15. A filtered render reports and removes only managed files outside its selected set.
16. Terrazzo can remove its repository-local sync script without changing the intended projection.
