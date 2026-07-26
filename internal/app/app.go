// Package app is the private application layer: object dispatch, flag
// parsing, orchestration, output, and exit codes for gh-render.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
)

const rootUsage = `Generate deterministic local projections of GitHub objects.

Usage:
  gh render <object> [flags]

Objects:
  issues    Render repository issues as Markdown

Run "gh render <object> --help" for object-specific help.
`

const issuesUsage = `Render repository issues as Markdown.

Usage:
  gh render issues [flags]

Flags:
      --repo owner/repo   Repository to read; defaults to the current repository
      --output directory  Output directory; defaults to .issues
      --check             Exit non-zero when rendered files are stale
      --dry-run           Report changes without writing files
      --state state       Filter by open, closed, or all; defaults to all
      --label name        Require a label; may be repeated
      --assignee login    Filter by assignee; accepts @me
      --author login      Filter by issue author; accepts @me
      --limit number      Keep the highest-ranked positive number of matches
      --sort field        Rank by updated, created, or number; defaults to updated
      --order direction   Rank ascending or descending; defaults to desc
`

// ErrProjectionStale is returned by an issues executor when --check finds the
// projection out of date. Run maps it to exit code 3.
var ErrProjectionStale = errors.New("projection is stale")

// IssuesExecutor performs a parsed issues command and returns its output.
type IssuesExecutor func(
	ctx context.Context,
	options IssuesCommandOptions,
) (IssuesCommandResult, error)

// IssuesCommandResult carries the user-facing stdout of one execution.
type IssuesCommandResult struct {
	Output string
}

// Dependencies is the application's injection seam for tests.
type Dependencies struct {
	ExecuteIssues IssuesExecutor
}

// DefaultDependencies binds the production executor.
func DefaultDependencies() Dependencies {
	return Dependencies{ExecuteIssues: executeIssues}
}

// Run dispatches the object command and returns the process exit code.
func Run(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	dependencies Dependencies,
) int {
	if len(arguments) == 0 || isHelp(arguments[0]) {
		fmt.Fprint(stdout, rootUsage)
		return 0
	}

	switch arguments[0] {
	case "issues":
		return runIssues(ctx, arguments[1:], stdout, stderr, dependencies.ExecuteIssues)
	default:
		fmt.Fprintf(stderr, "unknown object %q\n\n%s", arguments[0], rootUsage)
		return 2
	}
}

// runIssues parses issue flags, invokes the executor, and maps errors to the
// documented exit codes.
func runIssues(
	ctx context.Context,
	arguments []string,
	stdout io.Writer,
	stderr io.Writer,
	execute IssuesExecutor,
) int {
	if len(arguments) > 0 && isHelp(arguments[0]) {
		fmt.Fprint(stdout, issuesUsage)
		return 0
	}

	options, err := parseIssuesOptions(arguments)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n\n%s", err, issuesUsage)
		return 2
	}

	result, err := execute(ctx, options)
	if err != nil {
		if errors.Is(err, ErrProjectionStale) {
			fmt.Fprint(stdout, result.Output)
			return 3
		}
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprint(stdout, result.Output)
	return 0
}

func isHelp(argument string) bool {
	return argument == "-h" || argument == "--help" || argument == "help"
}
