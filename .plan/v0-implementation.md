---
title: "gh-render v0 detailed implementation"
date: 2026-07-26
status: approved
affects: "gh render issues and v0.1.0"
---

# gh-render v0 Detailed Implementation Plan

> Implement `gh render issues` with a private application layer, an issue-domain package, and a reusable projection writer. This document is the coding handoff. The behavioral authority remains `docs/spec.md` and `docs/objects/issues.md`.

## Table of Contents

1. [Outcome and constraints](#1-outcome-and-constraints)
2. [Source tree](#2-source-tree)
3. [Dependency direction](#3-dependency-direction)
4. [Root command layer](#4-root-command-layer)
5. [Issue domain model](#5-issue-domain-model)
6. [GitHub source](#6-github-source)
7. [Selection](#7-selection)
8. [Markdown rendering](#8-markdown-rendering)
9. [Projection planning and writes](#9-projection-planning-and-writes)
10. [Errors and output](#10-errors-and-output)
11. [Testing](#11-testing)
12. [Implementation sequence](#12-implementation-sequence)
13. [Live acceptance and migration](#13-live-acceptance-and-migration)
14. [Definition of done](#14-definition-of-done)

## 1. Outcome and constraints

The implementation is complete when:

```bash
gh render issues
gh render issues --limit 20
gh render issues --label bug --assignee @me
gh render issues --dry-run
gh render issues --check
```

all satisfy the approved specifications.

Implementation constraints:

1. Use Go's standard library and the existing `github.com/cli/go-gh/v2` dependency.
2. Use the GitHub REST issues endpoint. Do not shell out to `gh issue list`.
3. Keep GitHub transport, selection, rendering, and filesystem mutation independently testable.
4. Do not introduce Cobra, a configuration file, templates, or a public Go API in v0.
5. Do not write to GitHub.
6. Do not weaken unmanaged-file protection to ease the Terrazzo migration.
7. Do not commit generated `.issues/` until the extension can dogfood itself.
8. Keep every `_test.go` file and golden fixture under the top-level `tests/` directory.

When this plan conflicts with `docs/spec.md` or `docs/objects/issues.md`, stop and update the authoritative specification before changing behavior.

## 2. Source tree

Implement this tree:

```text
gh-render/
├── main.go
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   └── issues_command.go
│   ├── issues/
│   │   ├── model.go
│   │   ├── client.go
│   │   ├── select.go
│   │   └── render.go
│   └── projection/
│       ├── projection.go
│       └── writer.go
├── tests/
│   ├── app/
│   │   └── app_test.go
│   ├── issues/
│   │   ├── client_test.go
│   │   ├── select_test.go
│   │   ├── render_test.go
│   │   └── testdata/
│   │       ├── issue.golden.md
│   │       └── index.golden.md
│   └── projection/
│       ├── projection_test.go
│       └── writer_test.go
├── docs/
├── .plan/
└── CHANGELOG.md
```

Responsibilities:

| Location | Responsibility |
| --- | --- |
| `main.go` | Process entry point and signal-aware context. |
| `internal/app/app.go` | Root object dispatch, dependency seam, stdout/stderr, and exit codes. |
| `internal/app/issues_command.go` | Flag parsing, repository resolution, dependency construction, orchestration, user output. |
| `internal/issues/model.go` | Repository, issue, selection, and enum types. |
| `internal/issues/client.go` | Authenticated REST calls, pagination, API normalization. |
| `internal/issues/select.go` | Alias resolution, filtering, ranking, limiting, final ordering. |
| `internal/issues/render.go` | Deterministic issue and index Markdown. |
| `internal/projection/projection.go` | Expected-file, ownership, change, and plan types. |
| `internal/projection/writer.go` | Disk inventory, conflict detection, dry-run/check plan, atomic application. |
| `tests/` | Black-box application, package, golden, and filesystem tests. |

Do not create `internal/github`, `internal/select`, or `internal/render`. Those boundaries split one issue feature across packages and force the normalized issue type through unnecessary APIs.

Delete the scaffold's root `main_test.go` after its help and dispatch cases move to `tests/app/app_test.go`. Production directories contain no `_test.go` files.

## 3. Dependency direction

The package graph is:

```text
package main
└── internal/app

internal/app
├── go-gh/api
├── go-gh/repository
├── internal/issues
└── internal/projection

internal/issues
└── internal/projection    # File type only

internal/projection
└── standard library
```

`internal/issues` may return `projection.File` values from its renderer. It must not call the filesystem writer. `internal/app` owns orchestration:

```text
parse flags
→ resolve repository
→ construct REST client
→ fetch and normalize
→ resolve selection
→ select issues
→ render expected files
→ build projection plan
→ check, report, or apply
```

The packages under `tests/` may import all three `internal` packages because their import paths remain inside `github.com/digimata/gh-render`. They exercise exported internal contracts without creating a public library API.

Future object packages may reuse `internal/projection`. Shared GitHub transport should be extracted only after a second renderer demonstrates a real common API.

## 4. Application and command layer

### 4.1 — Process entry

Reduce `main.go` to process setup:

```go
func main() {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()

    os.Exit(app.Run(
        ctx,
        os.Args[1:],
        os.Stdout,
        os.Stderr,
        app.DefaultDependencies(),
    ))
}
```

Keep root help, unknown-object behavior, and issue-specific execution in `internal/app`. `package main` contains no business logic.

### 4.2 — Parsed options

Use the standard `flag` package with `flag.ContinueOnError`. Suppress the package's default output and return one consistent usage message.

```go
type IssuesCommandOptions struct {
    Repository string
    Output     string
    Check      bool
    DryRun     bool

    State      string
    Labels     []string
    Assignee   string
    Author     string
    Limit      int
    Sort       string
    Order      string
}
```

Defaults:

```text
Output = .issues
State  = all
Limit  = 0        # unlimited internally
Sort   = updated
Order  = desc
```

Implement a small `stringListFlag` satisfying `flag.Value` for repeated `--label`.

```go
type stringListFlag []string

func (values *stringListFlag) String() string
func (values *stringListFlag) Set(value string) error
```

`parseIssuesOptions` returns usage errors without touching GitHub or disk:

```go
func parseIssuesOptions(arguments []string) (IssuesCommandOptions, error)
```

Reject:

- positional arguments;
- unknown flags;
- `--check` with `--dry-run`;
- state outside `open|closed|all`;
- sort outside `updated|created|number`;
- order outside `asc|desc`;
- limit below zero;
- an explicitly supplied limit of zero;
- empty labels, assignees, or authors.

Because the default internal limit is zero, track whether `--limit` was present with a custom integer flag or a separate boolean.

### 4.3 — Repository resolution

Use the installed `go-gh` repository package:

```go
func resolveRepository(value string) (issues.Repository, error)
```

- Empty value calls `repository.Current()`.
- Nonempty value calls `repository.Parse(value)`.
- Convert the result into the local `issues.Repository`.
- Preserve `Host` for GitHub Enterprise authentication and API routing.

Relevant dependency sources:

- `github.com/cli/go-gh/v2/pkg/repository`
- `github.com/cli/go-gh/v2/pkg/api`

### 4.4 — Test seam

Do not inject every function independently. Export one executor seam from the internal application package:

```go
type IssuesExecutor func(
    ctx context.Context,
    options IssuesCommandOptions,
) (IssuesCommandResult, error)

type IssuesCommandResult struct {
    Output string
}

type Dependencies struct {
    ExecuteIssues IssuesExecutor
}

func DefaultDependencies() Dependencies

func Run(
    ctx context.Context,
    arguments []string,
    stdout io.Writer,
    stderr io.Writer,
    dependencies Dependencies,
) int
```

`DefaultDependencies` binds the unexported production `executeIssues`. Tests under `tests/app` pass `Dependencies` with a fake executor. This covers flag handling, help, exit codes, stdout, and stderr without importing `package main`, starting a subprocess, or touching network and disk.

For a stale check, `executeIssues` returns the deterministic change list in `IssuesCommandResult.Output` together with `ErrProjectionStale`. `Run` writes the result to stdout and maps the sentinel to exit `3` without formatting it as an operational error.

`executeIssues` uses concrete package implementations. Lower packages use their own narrow test seams.

These types are exported only because black-box tests need them. Go's `internal` rule prevents external modules from importing them.

## 5. Issue domain model

Create these types in `internal/issues/model.go`.

```go
package issues

import "time"

type Repository struct {
    Host  string
    Owner string
    Name  string
}

func (repository Repository) Slug() string
func (repository Repository) IssuesURL() string

type IssueState string

const (
    IssueOpen   IssueState = "open"
    IssueClosed IssueState = "closed"
)

type StateFilter string

const (
    StateAll    StateFilter = "all"
    StateOpen   StateFilter = "open"
    StateClosed StateFilter = "closed"
)

type SortField string

const (
    SortUpdated SortField = "updated"
    SortCreated SortField = "created"
    SortNumber  SortField = "number"
)

type SortOrder string

const (
    OrderAscending  SortOrder = "asc"
    OrderDescending SortOrder = "desc"
)

type Issue struct {
    Number     int
    Title      string
    Body       string
    State      IssueState
    Author     string
    URL        string
    Labels     []string
    Assignees  []string
    Milestone  *string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type Selection struct {
    State      StateFilter
    Labels     []string
    Assignee   string
    Author     string
    Limit      int
    Sort       SortField
    Order      SortOrder
}

type ResolvedSelection struct {
    State      StateFilter
    Labels     []string
    Assignee   string
    Author     string
    Limit      int
    Sort       SortField
    Order      SortOrder
}
```

`Selection` may contain `@me`. `ResolvedSelection` must contain only concrete logins. Empty assignee or author means any. A zero limit means unlimited.

Keep fields as values unless absence has distinct semantics. `Milestone` is a pointer because null and an empty title are different source values.

Normalize labels and assignees once at the API boundary:

- remove case-insensitive duplicates;
- sort the preserved GitHub spelling with bytewise lexical ordering;
- preserve GitHub's returned spelling.

Use case-insensitive comparison only for deduplication and selector matching. Rendering order follows the specification's lexical rule.

## 6. GitHub source

### 6.1 — REST client interface

Use `api.NewRESTClient(api.ClientOptions{Host: repository.Host, Timeout: 30 * time.Second})`.

Define the narrow interface consumed by `issues.Client`:

```go
type RESTDoer interface {
    DoWithContext(
        ctx context.Context,
        method string,
        path string,
        body io.Reader,
        response any,
    ) error
}

type Client struct {
    api RESTDoer
}

func NewClient(api RESTDoer) *Client
func (client *Client) FetchAll(ctx context.Context, repository Repository) ([]Issue, error)
func (client *Client) ViewerLogin(ctx context.Context) (string, error)
```

`*api.RESTClient` satisfies `RESTDoer`. Tests use a fake that records method and path and populates typed response values.

### 6.2 — Wire response

Keep REST JSON structs private to `client.go`:

```go
type apiIssue struct {
    Number    int             `json:"number"`
    Title     string          `json:"title"`
    Body      *string         `json:"body"`
    State     string          `json:"state"`
    HTMLURL   string          `json:"html_url"`
    User      apiUser         `json:"user"`
    Labels    []apiLabel      `json:"labels"`
    Assignees []apiUser       `json:"assignees"`
    Milestone *apiMilestone   `json:"milestone"`
    CreatedAt time.Time       `json:"created_at"`
    UpdatedAt time.Time       `json:"updated_at"`
    Pull      json.RawMessage `json:"pull_request"`
}

type apiUser struct {
    Login string `json:"login"`
}

type apiLabel struct {
    Name string `json:"name"`
}

type apiMilestone struct {
    Title string `json:"title"`
}
```

An absent `pull_request` field leaves `Pull` empty. A nonempty, non-null value means the record is a pull request and must be excluded.

### 6.3 — Pagination

Fetch:

```text
GET repos/{owner}/{repository}/issues?state=all&per_page=100&page=N&sort=created&direction=asc
```

Use `url.PathEscape` for owner and repository path segments and `url.Values` for the query.

Pagination rules:

1. Start at page 1.
2. Normalize only non-pull-request records.
3. Stop when the raw API page contains fewer than 100 records.
4. An exactly full final page causes one additional empty request.
5. Detect duplicate issue numbers across pages and return an error.
6. Check `ctx.Err()` before each request.

Use the raw page length for termination. A page containing pull requests may normalize to fewer than 100 issues but is not the last page.

### 6.4 — Viewer

Call `GET user` only when assignee or author equals `@me`. Normalize the returned login and reject an empty response.

Do not fetch the viewer for selections without aliases.

## 7. Selection

Implement pure functions in `internal/issues/select.go`:

```go
func ResolveSelection(
    selection Selection,
    viewerLogin string,
) (ResolvedSelection, error)

func NeedsViewer(selection Selection) bool

func Select(
    source []Issue,
    selection ResolvedSelection,
) []Issue
```

### 7.1 — Resolution

`ResolveSelection`:

1. validates enum values and limit;
2. replaces assignee or author `@me` with `viewerLogin`;
3. rejects `@me` when the viewer login is empty;
4. deduplicates and sorts labels;
5. returns a new value without mutating input.

### 7.2 — Filtering

Filter predicates:

- `StateAll` accepts both states.
- A repeated label selection requires every label.
- Assignee matches when any issue assignee equals the selected login.
- Author compares the issue author.
- Label and login comparisons use `strings.EqualFold`.

All predicate types combine with AND.

### 7.3 — Ranking and limiting

Rank a copied slice. Never mutate the source slice.

Primary comparisons:

- `SortUpdated`: `Issue.UpdatedAt`
- `SortCreated`: `Issue.CreatedAt`
- `SortNumber`: `Issue.Number`

The selected order applies to the primary comparison. Equal primary values compare issue number in the same order. Apply the positive limit after ranking.

Finally, sort the selected subset by ascending issue number. Rendering order is independent of ranking order.

Tests must use equal timestamps to prove deterministic issue-number tie-breaking.

## 8. Markdown rendering

### 8.1 — API

Implement:

```go
func Render(
    repository Repository,
    selection ResolvedSelection,
    selected []Issue,
) ([]projection.File, error)

func RenderIssue(issue Issue) ([]byte, error)

func RenderIndex(
    repository Repository,
    selection ResolvedSelection,
    selected []Issue,
) ([]byte, error)

func IsManagedName(name string) bool
func IsManagedContent(content []byte) bool
```

`Render` returns one index file and one file per selected issue. Return files sorted lexically by relative name even if the writer also sorts them.

### 8.2 — Projection file

Define in `internal/projection`:

```go
type File struct {
    Name    string
    Content []byte
}
```

Names are relative to the output root. The issues renderer emits only direct child names.

### 8.3 — YAML-compatible values

Use `encoding/json.Marshal` for frontmatter strings and string arrays. JSON string and array syntax is valid YAML and avoids hand-written escaping.

Do not add a YAML library.

Timestamps use:

```go
timestamp.UTC().Format(time.RFC3339)
```

### 8.4 — Body normalization

Preserve issue Markdown. Apply only:

1. convert CRLF to LF;
2. convert remaining standalone CR to LF;
3. remove trailing blank or whitespace-only lines;
4. preserve trailing spaces on the last nonblank content line;
5. end the generated file with one newline.

Do not parse or re-render the body.

### 8.5 — Index escaping

Implement helpers:

```go
func markdownTableCell(value string) string
func markdownInlineCode(value string) string
```

Table cells replace newlines with spaces and escape pipes. Inline code must select a backtick fence longer than any consecutive backtick run in the label.

Selection metadata uses:

- lowercase enum values;
- concrete assignee and author values;
- `any` for empty scalar selectors;
- `all` for an unlimited limit;
- a compact JSON array for labels;
- lexically sorted labels.

### 8.6 — Managed ownership

`IsManagedName` returns true only for:

- `index.md`; or
- `iss-\d{4,}\.md`.

`IsManagedContent` requires the exact marker:

```html
<!-- gh-render:managed -->
```

before the first top-level Markdown heading. This accepts the index marker at byte zero and the issue marker immediately after frontmatter while preventing a coincidental marker inside the user-authored issue body from granting ownership.

## 9. Projection planning and writes

### 9.1 — Types

Create these types in `internal/projection`:

```go
const ManagedMarker = "<!-- gh-render:managed -->"

type Ownership struct {
    IsManagedName    func(name string) bool
    IsManagedContent func(content []byte) bool
}

type ChangeKind uint8

const (
    Create ChangeKind = iota
    Update
    Remove
)

type Change struct {
    Kind ChangeKind
    Path string
}

type Plan struct {
    Root    string
    Files   []File
    Changes []Change
}

func BuildPlan(
    output string,
    expected []File,
    ownership Ownership,
) (Plan, error)

func (plan Plan) IsCurrent() bool
func (plan Plan) Apply() error
```

Keep any richer internal planned-file representation unexported.

### 9.2 — Expected-file validation

Before reading or writing targets:

1. require every file name to be relative;
2. reject empty, absolute, `.` or parent-traversing names;
3. reject duplicate cleaned names;
4. require every expected file to satisfy `ownership.IsManagedName`;
5. require every expected content value to satisfy `ownership.IsManagedContent`;
6. sort expected files by name.

### 9.3 — Output-root resolution

Resolve the output path to an absolute path. Canonicalize its nearest existing ancestor and reject symlink traversal that would place a target outside that root.

Normal mode may create the output directory with `0755`. `--check` and `--dry-run` must not create it. A missing output directory therefore produces:

- create changes in dry-run;
- stale status in check;
- directory and files in normal mode.

Do not use `os.Chdir`.

### 9.4 — Inventory and ownership

Read direct children only.

For every expected target:

- missing target → `Create`;
- regular managed target with different bytes → `Update`;
- regular managed target with equal bytes → no change;
- unmanaged target → fail the entire plan;
- directory or symlink target → fail the entire plan.

For every nonexpected direct child:

- managed name plus managed regular content → `Remove`;
- managed name plus unmanaged regular content → fail the entire plan;
- unmanaged name → ignore;
- directory → ignore;
- symlink → ignore and never follow.

Sort changes by path, then by kind for stable reporting.

### 9.5 — Apply

`Apply` operates only on a successfully built plan.

For create and update:

1. create a temporary file in the target directory;
2. write all bytes;
3. set mode `0644`;
4. sync and close;
5. rename over the target;
6. remove the temporary file on every failure path.

Do not remove an existing target to make rename succeed. If the platform cannot provide replacement through same-directory rename, return an error and preserve the old file.

Apply creates and updates before stale removals. Remove only the exact regular files recorded in the plan, using `Lstat` again immediately before deletion. Abort if their type or ownership changed since planning.

No recursive deletion is permitted.

## 10. Errors and output

### 10.1 — Exit mapping

Application command mapping:

| Condition | Exit |
| --- | --- |
| Success or current check | `0` |
| API, authentication, normalization, context, or filesystem failure | `1` |
| Invalid object, flag, selector, or combination | `2` |
| Stale check | `3` |

Export one internal-app sentinel so black-box application tests can return it from a fake executor:

```go
var ErrProjectionStale = errors.New("projection is stale")
```

Only a stale `--check` returns `ErrProjectionStale`. It may return the change list alongside that error. Do not infer staleness by matching error strings.

### 10.2 — Output

Usage errors:

```text
error: --limit must be greater than zero

<issues usage>
```

Operational errors:

```text
error: fetch owner/repository issues: <cause>
```

Dry-run output lists deterministic actions:

```text
create .issues/index.md
update .issues/iss-0002.md
remove .issues/iss-0009.md
```

Normal success prints one summary:

```text
Rendered 7 issues to .issues (2 created, 1 updated, 0 removed).
```

A current `--check` is silent. A stale check prints the same action list and exits `3`.

Wrap errors with operation and subject. Do not print tokens, response bodies containing credentials, or full debug dumps.

## 11. Testing

No automated test uses the network or the user's GitHub configuration.

### 11.1 — Application tests

Implement in `tests/app/app_test.go` with package `app_test`. Import `internal/app` and pass fake `app.Dependencies`.

Cover:

- root and issue help;
- every flag and default;
- repeated labels;
- unknown flags and positional arguments;
- invalid enums and limits;
- check/dry-run conflict;
- executor success;
- executor operational failure;
- stale check exit `3`;
- stdout/stderr separation;
- context cancellation propagation.

### 11.2 — Client tests

Implement in `tests/issues/client_test.go` with package `issues_test`. Use a fake `RESTDoer`.

Cover:

- repository path and query encoding;
- one page;
- multiple pages;
- exactly full final page followed by empty page;
- pull-request exclusion without premature pagination stop;
- null body and milestone;
- label and assignee normalization;
- duplicate issue number rejection;
- REST error wrapping;
- canceled context;
- viewer login success and empty-login failure.

### 11.3 — Selection tests

Implement in `tests/issues/select_test.go` with package `issues_test`. Table-test:

- each state;
- one and repeated labels;
- assignee and author;
- every cross-selector AND combination;
- case-insensitive matching;
- `@me` resolution;
- missing viewer;
- every sort field and order;
- equal-field number tie;
- limit before final serialization sort;
- unlimited selection;
- input immutability.

### 11.4 — Rendering tests

Implement in `tests/issues/render_test.go`. Store fixtures under `tests/issues/testdata/`.

Golden files cover:

- full issue file;
- full and filtered index;
- empty body;
- no labels, assignees, or milestone;
- quotes, backslashes, pipes, newlines, Unicode, and backticks;
- CRLF and trailing blank-line normalization;
- issue numbers over four digits;
- concrete `@me` replacement;
- stable label ordering;
- ownership detection before the first heading;
- rejection of marker text appearing only in an issue body.

Render the same input twice and compare bytes.

### 11.5 — Projection tests

Implement in `tests/projection/` with package `projection_test`. Use `t.TempDir`.

Every test imports only exported contracts from the relevant `internal` package. Test private helpers through their observable public behavior. Do not add test-only exports or place white-box tests beside production source.

Cover:

- missing output directory without mutation in check and dry-run;
- create, update, remove, and no-op;
- unmanaged expected target rejection;
- unmanaged matching-file rejection;
- unrelated unmanaged file preservation;
- expected directory and symlink rejection;
- stale managed file removal;
- stale directory and symlink preservation;
- duplicate and traversing expected names;
- deterministic change ordering;
- write failure cleanup;
- revalidation before removal;
- second application produces an empty plan;
- file modes and final bytes.

Run race detection before release:

```bash
go test -race ./...
```

## 12. Implementation sequence

Implement in this order. Keep each step buildable and tested.

### 12.1 — Models and CLI parsing

1. Add domain enums and structs.
2. Add options parsing and validation.
3. Move command execution into `internal/app`.
4. Reduce `main.go` to process setup.
5. Move the scaffold tests into `tests/app` and delete `main_test.go`.

Gate:

```bash
go test ./...
```

### 12.2 — Pure selection

1. Implement alias resolution.
2. Implement filtering.
3. Implement ranking, tie-breaking, limiting, and final ordering.
4. Complete the selection matrix tests.

### 12.3 — Pure rendering

1. Add `projection.File`.
2. Implement issue rendering.
3. Implement index and normalized selection rendering.
4. Add golden fixtures and ownership helpers.

### 12.4 — Projection writer

1. Validate expected files.
2. Inventory disk state and build a complete plan.
3. Implement dry-run/check reporting.
4. Implement safe application and stale removal.
5. Complete filesystem failure tests before integration.

### 12.5 — GitHub client

1. Implement REST wire types.
2. Implement pagination and normalization.
3. Implement viewer lookup.
4. Complete fake-client tests.

### 12.6 — Orchestration

1. Resolve repository.
2. Create host-aware REST client.
3. Fetch issues and viewer when required.
4. Resolve and apply selection.
5. Render expected files.
6. Build and process projection plan.
7. Map errors and output to the command contract.

### 12.7 — Documentation and release preparation

1. Remove pre-release and stub language from README and command help.
2. Add complete command examples.
3. Update `CHANGELOG.md`.
4. Confirm release workflow and module path.

Do not tag or release until live acceptance passes.

## 13. Live acceptance and migration

### 13.1 — Build local extension

```bash
go build -o gh-render .
gh extension install . --force
```

### 13.2 — Compare with Terrazzo

From the Terrazzo repository, render to a temporary directory outside `.issues/`:

```bash
gh render issues --output <temporary-directory>
```

Compare:

- seven issue files exist;
- titles, bodies, URLs, state, labels, assignees, milestone, and timestamps match;
- expected differences are limited to the new ownership marker and selection metadata;
- a second render changes nothing.

Then exercise:

```bash
gh render issues --output <temporary-directory> --limit 3
gh render issues --output <temporary-directory> --state open --assignee @me
gh render issues --output <temporary-directory> --dry-run
gh render issues --output <temporary-directory> --check
```

The `--limit 3` run should remove four managed snapshots from the temporary projection and keep the three most recently updated issues.

### 13.3 — Migrate Terrazzo

After comparison approval:

1. remove `scripts/sync-github-issues.mjs`;
2. remove `pnpm issues:pull`;
3. remove the verified legacy-generated `.issues/` files;
4. run `gh render issues`;
5. run it again and confirm no diff;
6. run `gh render issues --check`;
7. run Terrazzo's repository checks;
8. commit the migration separately from the extension implementation.

## 14. Definition of done

Claude should hand back:

1. the complete source tree in section 2;
2. passing `go test ./...`;
3. passing `go test -race ./...`;
4. passing `go vet ./...`;
5. a successful local build;
6. reviewed golden fixtures;
7. a clean second render against Terrazzo;
8. demonstrated full, limited, label, and `@me` projections;
9. updated README and changelog;
10. no release tag and no Terrazzo migration until separately approved.

The implementation is not complete if it passes unit tests but overwrites an unmanaged file, treats pull requests as issues, produces unstable bytes, or cannot explain a filtered projection from its index.
