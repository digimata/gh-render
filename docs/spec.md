---
title: "gh-render specification"
status: approved
version: "0.1"
date: 2026-07-26
---

# gh-render Specification

> `gh render` materializes deterministic, read-only local projections of GitHub objects. GitHub is authoritative; generated files are disposable views protected by explicit ownership and filesystem-safety rules.

## Table of Contents

1. [Purpose](#1-purpose)
2. [Vocabulary](#2-vocabulary)
3. [Command model](#3-command-model)
4. [Global rendering contract](#4-global-rendering-contract)
5. [Filesystem safety](#5-filesystem-safety)
6. [Validation modes](#6-validation-modes)
7. [Exit codes](#7-exit-codes)
8. [Renderer extension contract](#8-renderer-extension-contract)

## 1. Purpose

GitHub objects are useful inside a repository's local documentation and agent workflows, but GitHub remains their durable system of record. `gh render` closes that gap by deriving reviewable Markdown files from GitHub without creating a second writable tracker.

The extension reads GitHub and writes local files. It never creates, edits, closes, labels, or deletes GitHub objects.

## 2. Vocabulary

| Term | Meaning |
| --- | --- |
| Object | A supported GitHub collection such as issues. |
| Renderer | The object-specific fetch, normalization, and serialization implementation. |
| Projection | The complete local file set derived from one object collection. |
| Managed file | A file containing the exact `gh-render` ownership marker. |
| Unmanaged file | Any file without that marker, regardless of its filename. |
| Stale file | A managed file whose expected content differs from disk or whose source object no longer exists. |

## 3. Command model

The root grammar is:

```text
gh render <object> [flags]
```

`gh render` and `gh render --help` display root help. Unknown objects return a usage error. Each object owns its specific flags and output schema.

Every renderer supports these common flags:

| Flag | Meaning |
| --- | --- |
| `--repo owner/repo` | Read from an explicit repository instead of resolving the current repository. |
| `--output <directory>` | Write to an explicit directory instead of the renderer default. |
| `--check` | Perform no writes and fail when the projection differs from disk. |
| `--dry-run` | Perform no writes and report the files that would change. |

`--check` and `--dry-run` are mutually exclusive. Repository resolution follows the authenticated GitHub CLI context and produces a clear error when no repository can be resolved.

## 4. Global rendering contract

Every renderer must satisfy these invariants:

1. GitHub is the sole upstream authority.
2. Rendering is one-way. No command writes to GitHub.
3. Identical normalized GitHub data and arguments produce byte-identical files.
4. Generated output contains no render timestamp, host-specific path, random value, or unstable ordering.
5. Text files use UTF-8, LF line endings, and one final newline.
6. Records and collection fields use explicit deterministic ordering.
7. Every generated file contains this marker near its beginning:

   ```html
   <!-- gh-render:managed -->
   ```

8. A renderer computes and validates its complete write plan before changing disk state.
9. A successful second render against unchanged GitHub data produces no file changes.

## 5. Filesystem safety

A renderer may create a missing output directory. It may create a missing target file and replace an existing managed target file.

A renderer must refuse the entire operation before writing when:

- a target path exists and is unmanaged;
- an expected directory path is a file;
- a target resolves outside the canonical output directory;
- two records resolve to the same target;
- the output directory cannot be canonicalized safely.

Each file replacement uses a temporary file in the target directory followed by an atomic rename. Temporary files are removed after failure when possible.

Stale deletion is restricted to regular files that:

1. are directly owned by the active renderer;
2. match that renderer's documented filename pattern; and
3. contain the ownership marker.

The renderer does not recursively delete directories or follow symlinks during cleanup.

## 6. Validation modes

Normal mode writes the planned projection and reports a concise summary.

`--dry-run` fetches and renders the full projection, performs all conflict checks, and prints the paths that would be created, replaced, or removed. It does not modify the filesystem.

`--check` performs the same read and validation work. It prints a concise stale summary and exits with the stale-projection code when disk differs from the expected projection. It produces no output and exits successfully when the projection is current.

Neither mode weakens unmanaged-file or path-safety checks.

## 7. Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Rendering succeeded, or `--check` found the projection current. |
| `1` | Authentication, network, API, normalization, or filesystem failure. |
| `2` | Invalid command, object, flag, or flag combination. |
| `3` | `--check` found a stale projection. |

Errors go to stderr. Normal summaries and dry-run plans go to stdout. Errors identify the failed operation and relevant repository or path without exposing credentials.

## 8. Renderer extension contract

Each object renderer must define:

1. GitHub inclusion and exclusion rules.
2. Pagination behavior.
3. A normalized record model independent of API response structs.
4. Stable record and collection ordering.
5. Default output directory.
6. Managed filename patterns.
7. File schemas and examples.
8. Stale-file ownership rules.
9. Object-specific flags.
10. Golden rendering and filesystem-safety tests.

Object specifications live under `docs/objects/`. A new renderer requires an approved object specification before implementation.
