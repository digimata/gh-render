---
id: 7
title: "Make file imports atomically non-overwriting"
status: open
source: github
github_url: "https://github.com/octo/repo/issues/7"
labels: ["bug","p0"]
assignees: ["dremnik"]
milestone: "v1.0"
created_at: "2026-07-26T19:37:25Z"
updated_at: "2026-07-26T20:00:00Z"
---
<!-- gh-render:managed -->

# ISS-0007 — Make file imports atomically non-overwriting

GitHub: [#7 — Make file imports atomically non-overwriting](https://github.com/octo/repo/issues/7)

Imports must never clobber files.

- write to a temporary file
- rename into place
