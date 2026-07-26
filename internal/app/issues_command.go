package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"

	"github.com/digimata/gh-render/internal/issues"
	"github.com/digimata/gh-render/internal/projection"
)

// IssuesCommandOptions is the parsed and validated issues command line.
type IssuesCommandOptions struct {
	Repository string
	Output     string
	Check      bool
	DryRun     bool

	State    string
	Labels   []string
	Assignee string
	Author   string
	Limit    int
	Sort     string
	Order    string
}

// stringListFlag collects repeated flag values, rejecting empty ones.
type stringListFlag []string

func (values *stringListFlag) String() string {
	return strings.Join(*values, ",")
}

func (values *stringListFlag) Set(value string) error {
	if value == "" {
		return errors.New("must not be empty")
	}
	*values = append(*values, value)
	return nil
}

// limitFlag parses --limit, tracking presence so an explicit zero is a usage
// error while the absent default means unlimited.
type limitFlag struct {
	value int
}

func (limit *limitFlag) String() string {
	if limit == nil || limit.value == 0 {
		return ""
	}
	return strconv.Itoa(limit.value)
}

func (limit *limitFlag) Set(value string) error {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a positive integer, got %q", value)
	}
	if parsed <= 0 {
		return errors.New("must be greater than zero")
	}
	limit.value = parsed
	return nil
}

// parseIssuesOptions parses and validates the issues command line without
// touching GitHub or disk.
func parseIssuesOptions(arguments []string) (IssuesCommandOptions, error) {
	options := IssuesCommandOptions{}
	flags := flag.NewFlagSet("issues", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {}

	var labels stringListFlag
	var limit limitFlag
	flags.StringVar(&options.Repository, "repo", "", "")
	flags.StringVar(&options.Output, "output", ".issues", "")
	flags.BoolVar(&options.Check, "check", false, "")
	flags.BoolVar(&options.DryRun, "dry-run", false, "")
	flags.StringVar(&options.State, "state", "all", "")
	flags.Var(&labels, "label", "")
	flags.StringVar(&options.Assignee, "assignee", "", "")
	flags.StringVar(&options.Author, "author", "", "")
	flags.Var(&limit, "limit", "")
	flags.StringVar(&options.Sort, "sort", "updated", "")
	flags.StringVar(&options.Order, "order", "desc", "")

	if err := flags.Parse(arguments); err != nil {
		return options, err
	}
	options.Labels = labels
	options.Limit = limit.value

	if flags.NArg() > 0 {
		return options, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	if options.Check && options.DryRun {
		return options, errors.New("--check and --dry-run are mutually exclusive")
	}
	switch options.State {
	case "open", "closed", "all":
	default:
		return options, fmt.Errorf("--state must be open, closed, or all, got %q", options.State)
	}
	switch options.Sort {
	case "updated", "created", "number":
	default:
		return options, fmt.Errorf("--sort must be updated, created, or number, got %q", options.Sort)
	}
	switch options.Order {
	case "asc", "desc":
	default:
		return options, fmt.Errorf("--order must be asc or desc, got %q", options.Order)
	}

	var emptySelector error
	flags.Visit(func(present *flag.Flag) {
		if emptySelector != nil {
			return
		}
		switch present.Name {
		case "assignee":
			if options.Assignee == "" {
				emptySelector = errors.New("--assignee must not be empty")
			}
		case "author":
			if options.Author == "" {
				emptySelector = errors.New("--author must not be empty")
			}
		case "output":
			if options.Output == "" {
				emptySelector = errors.New("--output must not be empty")
			}
		}
	})
	return options, emptySelector
}

// resolveRepository resolves the current or explicitly named repository
// through the authenticated GitHub CLI context.
func resolveRepository(value string) (issues.Repository, error) {
	if value == "" {
		current, err := repository.Current()
		if err != nil {
			return issues.Repository{}, fmt.Errorf("resolve current repository: %w", err)
		}
		return issues.Repository{Host: current.Host, Owner: current.Owner, Name: current.Name}, nil
	}
	parsed, err := repository.Parse(value)
	if err != nil {
		return issues.Repository{}, fmt.Errorf("resolve repository %q: %w", value, err)
	}
	return issues.Repository{Host: parsed.Host, Owner: parsed.Owner, Name: parsed.Name}, nil
}

// executeIssues is the production issues executor: it fetches, selects,
// renders, and applies or reports the projection plan.
func executeIssues(ctx context.Context, options IssuesCommandOptions) (IssuesCommandResult, error) {
	repo, err := resolveRepository(options.Repository)
	if err != nil {
		return IssuesCommandResult{}, err
	}

	restClient, err := api.NewRESTClient(api.ClientOptions{
		Host:    repo.Host,
		Timeout: 30 * time.Second,
	})
	if err != nil {
		return IssuesCommandResult{}, fmt.Errorf("create GitHub client for %s: %w", repo.Host, err)
	}
	client := issues.NewClient(restClient)

	selection := issues.Selection{
		State:    issues.StateFilter(options.State),
		Labels:   options.Labels,
		Assignee: options.Assignee,
		Author:   options.Author,
		Limit:    options.Limit,
		Sort:     issues.SortField(options.Sort),
		Order:    issues.SortOrder(options.Order),
	}

	viewer := ""
	if issues.NeedsViewer(selection) {
		viewer, err = client.ViewerLogin(ctx)
		if err != nil {
			return IssuesCommandResult{}, err
		}
	}
	resolved, err := issues.ResolveSelection(selection, viewer)
	if err != nil {
		return IssuesCommandResult{}, err
	}

	all, err := client.FetchAll(ctx, repo)
	if err != nil {
		return IssuesCommandResult{}, err
	}
	selected := issues.Select(all, resolved)

	files, err := issues.Render(repo, resolved, selected)
	if err != nil {
		return IssuesCommandResult{}, fmt.Errorf("render %s issues: %w", repo.Slug(), err)
	}

	plan, err := projection.BuildPlan(options.Output, files, projection.Ownership{
		IsManagedName:    issues.IsManagedName,
		IsManagedContent: issues.IsManagedContent,
	})
	if err != nil {
		return IssuesCommandResult{}, err
	}

	switch {
	case options.DryRun:
		return IssuesCommandResult{Output: formatChanges(plan.Changes)}, nil
	case options.Check:
		if plan.IsCurrent() {
			return IssuesCommandResult{}, nil
		}
		return IssuesCommandResult{Output: formatChanges(plan.Changes)}, ErrProjectionStale
	default:
		if err := plan.Apply(); err != nil {
			return IssuesCommandResult{}, err
		}
		return IssuesCommandResult{Output: renderSummary(len(selected), options.Output, plan.Changes)}, nil
	}
}

// formatChanges lists planned actions one per line in stable order.
func formatChanges(changes []projection.Change) string {
	var builder strings.Builder
	for _, change := range changes {
		fmt.Fprintf(&builder, "%s %s\n", change.Kind, change.Path)
	}
	return builder.String()
}

// renderSummary formats the one-line normal-mode summary.
func renderSummary(issueCount int, output string, changes []projection.Change) string {
	created, updated, removed := 0, 0, 0
	for _, change := range changes {
		switch change.Kind {
		case projection.Create:
			created++
		case projection.Update:
			updated++
		case projection.Remove:
			removed++
		}
	}
	return fmt.Sprintf(
		"Rendered %d issues to %s (%d created, %d updated, %d removed).\n",
		issueCount, output, created, updated, removed,
	)
}
