package cli

import (
	"flag"
	"fmt"
)

func runList(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	all := fs.Bool("all", false, "Include deleted issues")
	status := fs.String("status", "", "Filter by status (open, in_progress, closed, deleted)")
	priority := fs.String("priority", "", "Filter by priority")

	if err := fs.Parse(args); err != nil {
		return err
	}

	client := newClient(gf)
	repo := resolveRepo(gf)

	issues, err := client.ListIssues(repo, ListOpts{
		Status:   *status,
		Priority: *priority,
		All:      *all,
	})
	if err != nil {
		return fmt.Errorf("list issues: %w", err)
	}

	printIssueList(issues, gf.pretty)
	return nil
}
