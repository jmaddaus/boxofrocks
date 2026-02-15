package cli

import (
	"flag"
	"fmt"
	"strconv"
)

func runUpdate(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	status := fs.String("status", "", "New status (open, in_progress, blocked, in_review, closed)")
	priority := fs.Int("priority", -1, "New priority")
	title := fs.String("title", "", "New title")
	description := fs.String("description", "", "New description")

	if err := fs.Parse(args); err != nil {
		return err
	}

	remaining := fs.Args()
	if len(remaining) == 0 {
		return fmt.Errorf("usage: bor update <id> [--status S] [--priority N] [--title T] [--description D]")
	}

	id, err := strconv.Atoi(remaining[0])
	if err != nil {
		return fmt.Errorf("invalid issue id %q: %w", remaining[0], err)
	}

	fields := make(map[string]interface{})
	if *status != "" {
		fields["status"] = *status
	}
	if *priority >= 0 {
		fields["priority"] = *priority
	}
	if *title != "" {
		fields["title"] = *title
	}
	if *description != "" {
		fields["description"] = *description
	}

	if len(fields) == 0 {
		return fmt.Errorf("no fields to update; use --status, --priority, --title, or --description")
	}

	client := newClient(gf)

	issue, err := client.UpdateIssue(id, fields)
	if err != nil {
		return fmt.Errorf("update issue: %w", err)
	}

	printIssue(issue, gf.pretty)
	return nil
}
