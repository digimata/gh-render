// Package app_test black-box tests the application layer: dispatch, help,
// flag parsing, exit codes, and stream separation, using a fake executor.
package app_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/digimata/gh-render/internal/app"
)

// run invokes app.Run with a fake executor and captured streams.
func run(arguments []string, execute app.IssuesExecutor) (code int, stdout, stderr string) {
	var outBuffer, errBuffer bytes.Buffer
	code = app.Run(
		context.Background(),
		arguments,
		&outBuffer,
		&errBuffer,
		app.Dependencies{ExecuteIssues: execute},
	)
	return code, outBuffer.String(), errBuffer.String()
}

// forbidExecution returns an executor that fails the test when called.
func forbidExecution(t *testing.T) app.IssuesExecutor {
	t.Helper()
	return func(context.Context, app.IssuesCommandOptions) (app.IssuesCommandResult, error) {
		t.Error("executor must not be called")
		return app.IssuesCommandResult{}, nil
	}
}

// defaultOptions is the expected parse result of a bare "issues" invocation.
func defaultOptions() app.IssuesCommandOptions {
	return app.IssuesCommandOptions{
		Output: ".issues",
		State:  "all",
		Sort:   "updated",
		Order:  "desc",
	}
}

func TestRootHelp(t *testing.T) {
	for _, arguments := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		t.Run(strings.Join(arguments, " "), func(t *testing.T) {
			code, stdout, stderr := run(arguments, forbidExecution(t))
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if !strings.Contains(stdout, "gh render <object>") {
				t.Errorf("stdout missing root usage:\n%s", stdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestIssuesHelp(t *testing.T) {
	for _, arguments := range [][]string{{"issues", "--help"}, {"issues", "-h"}, {"issues", "help"}} {
		t.Run(strings.Join(arguments, " "), func(t *testing.T) {
			code, stdout, stderr := run(arguments, forbidExecution(t))
			if code != 0 {
				t.Errorf("exit code = %d, want 0", code)
			}
			if !strings.Contains(stdout, "gh render issues [flags]") {
				t.Errorf("stdout missing issues usage:\n%s", stdout)
			}
			if stderr != "" {
				t.Errorf("stderr = %q, want empty", stderr)
			}
		})
	}
}

func TestUnknownObject(t *testing.T) {
	code, stdout, stderr := run([]string{"widgets"}, forbidExecution(t))
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, `unknown object "widgets"`) {
		t.Errorf("stderr missing unknown-object message:\n%s", stderr)
	}
	if !strings.Contains(stderr, "gh render <object>") {
		t.Errorf("stderr missing root usage:\n%s", stderr)
	}
}

func TestIssuesFlagParsing(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		mutate    func(*app.IssuesCommandOptions)
	}{
		{"defaults", nil, func(*app.IssuesCommandOptions) {}},
		{"repo", []string{"--repo", "octo/repo"}, func(options *app.IssuesCommandOptions) {
			options.Repository = "octo/repo"
		}},
		{"output", []string{"--output", "docs/issues"}, func(options *app.IssuesCommandOptions) {
			options.Output = "docs/issues"
		}},
		{"check", []string{"--check"}, func(options *app.IssuesCommandOptions) {
			options.Check = true
		}},
		{"dry run", []string{"--dry-run"}, func(options *app.IssuesCommandOptions) {
			options.DryRun = true
		}},
		{"state open", []string{"--state", "open"}, func(options *app.IssuesCommandOptions) {
			options.State = "open"
		}},
		{"state closed", []string{"--state", "closed"}, func(options *app.IssuesCommandOptions) {
			options.State = "closed"
		}},
		{"one label", []string{"--label", "bug"}, func(options *app.IssuesCommandOptions) {
			options.Labels = []string{"bug"}
		}},
		{"repeated labels", []string{"--label", "bug", "--label", "p0"}, func(options *app.IssuesCommandOptions) {
			options.Labels = []string{"bug", "p0"}
		}},
		{"assignee", []string{"--assignee", "octocat"}, func(options *app.IssuesCommandOptions) {
			options.Assignee = "octocat"
		}},
		{"assignee alias", []string{"--assignee", "@me"}, func(options *app.IssuesCommandOptions) {
			options.Assignee = "@me"
		}},
		{"author", []string{"--author", "@me"}, func(options *app.IssuesCommandOptions) {
			options.Author = "@me"
		}},
		{"limit", []string{"--limit", "20"}, func(options *app.IssuesCommandOptions) {
			options.Limit = 20
		}},
		{"sort created", []string{"--sort", "created"}, func(options *app.IssuesCommandOptions) {
			options.Sort = "created"
		}},
		{"sort number", []string{"--sort", "number"}, func(options *app.IssuesCommandOptions) {
			options.Sort = "number"
		}},
		{"order asc", []string{"--order", "asc"}, func(options *app.IssuesCommandOptions) {
			options.Order = "asc"
		}},
		{"all flags combined", []string{
			"--repo", "octo/repo", "--output", "out", "--dry-run",
			"--state", "open", "--label", "bug", "--label", "p0",
			"--assignee", "@me", "--author", "octocat",
			"--limit", "5", "--sort", "number", "--order", "asc",
		}, func(options *app.IssuesCommandOptions) {
			options.Repository = "octo/repo"
			options.Output = "out"
			options.DryRun = true
			options.State = "open"
			options.Labels = []string{"bug", "p0"}
			options.Assignee = "@me"
			options.Author = "octocat"
			options.Limit = 5
			options.Sort = "number"
			options.Order = "asc"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var received app.IssuesCommandOptions
			calls := 0
			execute := func(_ context.Context, options app.IssuesCommandOptions) (app.IssuesCommandResult, error) {
				calls++
				received = options
				return app.IssuesCommandResult{}, nil
			}

			code, _, stderr := run(append([]string{"issues"}, test.arguments...), execute)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %s)", code, stderr)
			}
			if calls != 1 {
				t.Fatalf("executor calls = %d, want 1", calls)
			}
			expected := defaultOptions()
			test.mutate(&expected)
			if !reflect.DeepEqual(received, expected) {
				t.Errorf("options = %+v, want %+v", received, expected)
			}
		})
	}
}

func TestIssuesUsageErrors(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		fragment  string
	}{
		{"unknown flag", []string{"--bogus"}, "flag provided but not defined"},
		{"positional argument", []string{"extra"}, `unexpected argument "extra"`},
		{"trailing positional", []string{"--state", "open", "extra"}, `unexpected argument "extra"`},
		{"invalid state", []string{"--state", "merged"}, "--state must be open, closed, or all"},
		{"invalid sort", []string{"--sort", "priority"}, "--sort must be updated, created, or number"},
		{"invalid order", []string{"--order", "up"}, "--order must be asc or desc"},
		{"limit zero", []string{"--limit", "0"}, "must be greater than zero"},
		{"limit negative", []string{"--limit", "-3"}, "must be greater than zero"},
		{"limit nonnumeric", []string{"--limit", "many"}, "must be a positive integer"},
		{"empty label", []string{"--label", ""}, "must not be empty"},
		{"empty assignee", []string{"--assignee", ""}, "--assignee must not be empty"},
		{"empty author", []string{"--author", ""}, "--author must not be empty"},
		{"empty output", []string{"--output", ""}, "--output must not be empty"},
		{"check with dry run", []string{"--check", "--dry-run"}, "--check and --dry-run are mutually exclusive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr := run(append([]string{"issues"}, test.arguments...), forbidExecution(t))
			if code != 2 {
				t.Errorf("exit code = %d, want 2", code)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, "error: ") {
				t.Errorf("stderr missing error prefix:\n%s", stderr)
			}
			if !strings.Contains(stderr, test.fragment) {
				t.Errorf("stderr missing %q:\n%s", test.fragment, stderr)
			}
			if !strings.Contains(stderr, "gh render issues [flags]") {
				t.Errorf("stderr missing issues usage:\n%s", stderr)
			}
		})
	}
}

func TestExecutorSuccessWritesStdout(t *testing.T) {
	summary := "Rendered 7 issues to .issues (2 created, 1 updated, 0 removed).\n"
	execute := func(context.Context, app.IssuesCommandOptions) (app.IssuesCommandResult, error) {
		return app.IssuesCommandResult{Output: summary}, nil
	}

	code, stdout, stderr := run([]string{"issues"}, execute)
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if stdout != summary {
		t.Errorf("stdout = %q, want %q", stdout, summary)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestExecutorFailureWritesStderr(t *testing.T) {
	execute := func(context.Context, app.IssuesCommandOptions) (app.IssuesCommandResult, error) {
		return app.IssuesCommandResult{}, errors.New("fetch octo/repo issues: boom")
	}

	code, stdout, stderr := run([]string{"issues"}, execute)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr != "error: fetch octo/repo issues: boom\n" {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestStaleCheckExitsThree(t *testing.T) {
	changes := "remove .issues/iss-0009.md\nupdate .issues/index.md\n"
	execute := func(context.Context, app.IssuesCommandOptions) (app.IssuesCommandResult, error) {
		return app.IssuesCommandResult{Output: changes},
			fmt.Errorf("check projection: %w", app.ErrProjectionStale)
	}

	code, stdout, stderr := run([]string{"issues", "--check"}, execute)
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if stdout != changes {
		t.Errorf("stdout = %q, want %q", stdout, changes)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

type contextMarker struct{}

func TestContextReachesExecutor(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextMarker{}, "sentinel")
	ctx, cancel := context.WithCancel(ctx)
	cancel()

	calls := 0
	execute := func(received context.Context, _ app.IssuesCommandOptions) (app.IssuesCommandResult, error) {
		calls++
		if value, _ := received.Value(contextMarker{}).(string); value != "sentinel" {
			t.Errorf("context value = %q, want %q", value, "sentinel")
		}
		if !errors.Is(received.Err(), context.Canceled) {
			t.Errorf("context error = %v, want %v", received.Err(), context.Canceled)
		}
		return app.IssuesCommandResult{}, received.Err()
	}

	var outBuffer, errBuffer bytes.Buffer
	code := app.Run(ctx, []string{"issues"}, &outBuffer, &errBuffer, app.Dependencies{ExecuteIssues: execute})
	if calls != 1 {
		t.Fatalf("executor calls = %d, want 1", calls)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
