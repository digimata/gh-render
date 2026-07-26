package main

import (
	"fmt"
	"io"
	"os"
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

Planned flags:
      --repo owner/repo    Repository to read; defaults to the current repository
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

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) == 0 || isHelp(arguments[0]) {
		fmt.Fprint(stdout, rootUsage)
		return 0
	}

	switch arguments[0] {
	case "issues":
		return runIssues(arguments[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown object %q\n\n%s", arguments[0], rootUsage)
		return 2
	}
}

func runIssues(arguments []string, stdout, stderr io.Writer) int {
	if len(arguments) > 0 && isHelp(arguments[0]) {
		fmt.Fprint(stdout, issuesUsage)
		return 0
	}

	fmt.Fprintln(stderr, "the issues renderer is not implemented yet")
	return 1
}

func isHelp(argument string) bool {
	return argument == "-h" || argument == "--help" || argument == "help"
}
