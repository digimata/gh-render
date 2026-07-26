---
id: ADR-001
title: "One-way deterministic rendering"
status: accepted
date: 2026-07-26
---

# ADR-001 — One-way Deterministic Rendering

## 1. Context

Repository work often depends on GitHub issues, but issue data is unavailable to local documentation tools and agents unless fetched on demand. Copying issues into Markdown improves local access but risks creating a second writable tracker whose state diverges from GitHub.

Repository-local scripts can generate snapshots, but they duplicate implementation and runtime dependencies across codebases.

## 2. Decision

`gh-render` is a GitHub CLI extension that reads GitHub objects and materializes deterministic local Markdown projections.

GitHub remains authoritative. Rendering is strictly one-way. Generated files contain an ownership marker, and the extension replaces or removes only marked files within the active renderer's documented namespace.

The extension excludes render timestamps and other unstable values. Identical normalized source data produces byte-identical output. Each object has an approved specification under `docs/objects/`.

## 3. Alternatives considered

### 3.1 — Repository-local scripts

This works for one repository and provided the Terrazzo prototype. It duplicates code, commands, tests, and runtime assumptions across repositories.

### 3.2 — Bidirectional synchronization

This would allow editing local Markdown and pushing changes to GitHub. It introduces conflict resolution, permissions, destructive updates, and unclear authority. The added complexity does not serve the local-read use case.

### 3.3 — Runtime-only GitHub queries

Agents and tools could query GitHub whenever issue context is needed. This requires network access, repeats API work, prevents ordinary repository search and linking, and leaves no reviewable local artifact.

## 4. Consequences

- Repositories gain searchable and linkable GitHub projections.
- One installed extension replaces repository-local sync scripts.
- Generated files can be committed and reviewed without becoming writable source records.
- Consumers must render again to observe upstream changes.
- The extension must treat filesystem ownership and deterministic serialization as compatibility contracts.
- New GitHub object types require explicit specifications and tests rather than ad hoc output.
