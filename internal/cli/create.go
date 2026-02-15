package cli

import (
	"flag"
	"fmt"
)

func runCreate(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	priority := fs.Int("p", 0, "Priority (lower is higher priority)")
	issueType := fs.String("t", "task", "Issue type (task, bug, feature)")
	description := fs.String("d", "", "Description")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("usage: bor create \"title\" [-p priority] [-t type] [-d description]")
	}
	title := remaining[0]

	client := newClient(gf)
	repo := resolveRepo(gf)

	req := CreateIssueRequest{
		Title:       title,
		Description: *description,
		IssueType:   *issueType,
	}
	if *priority != 0 {
		req.Priority = priority
	}

	issue, err := client.CreateIssue(repo, req)
	if err != nil {
		return fmt.Errorf("create issue: %w", err)
	}

	printIssue(issue, gf.pretty)
	return nil
}
