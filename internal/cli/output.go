package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/jmaddaus/boxofrocks/internal/model"
)

// printIssue prints a single issue either as JSON or as a pretty-printed block.
func printIssue(issue *model.Issue, pretty bool) {
	if pretty {
		printPrettyIssue(issue)
		return
	}
	printJSON(issue)
}

// printIssueList prints a list of issues either as JSON or as a pretty-printed table.
func printIssueList(issues []*model.Issue, pretty bool) {
	if pretty {
		printPretty(issues)
		return
	}
	printJSON(issues)
}

// printJSON outputs v as compact JSON to stdout.
func printJSON(v interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

// printPretty outputs issues as a tabwriter-formatted table.
func printPretty(issues []*model.Issue) {
	if len(issues) == 0 {
		fmt.Println("No issues found.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tPRI\tTYPE\tOWNER\tTITLE")
	for _, iss := range issues {
		fmt.Fprintf(w, "#%d\t%s\t%d\t%s\t%s\t%s\n",
			iss.ID,
			iss.Status,
			iss.Priority,
			iss.IssueType,
			iss.Owner,
			iss.Title,
		)
	}
	w.Flush()
}

// printPrettyIssue outputs a single issue in a readable multi-line format.
func printPrettyIssue(issue *model.Issue) {
	if issue.GitHubID != nil {
		fmt.Printf("Issue #%d (GitHub #%d)\n", issue.ID, *issue.GitHubID)
	} else {
		fmt.Printf("Issue #%d\n", issue.ID)
	}
	fmt.Printf("  Title:       %s\n", issue.Title)
	fmt.Printf("  Status:      %s\n", issue.Status)
	fmt.Printf("  Priority:    %d\n", issue.Priority)
	fmt.Printf("  Type:        %s\n", issue.IssueType)
	if issue.Owner != "" {
		fmt.Printf("  Owner:       %s\n", issue.Owner)
	}
	if issue.Description != "" {
		fmt.Printf("  Description: %s\n", issue.Description)
	}
	if len(issue.Labels) > 0 {
		fmt.Printf("  Labels:      %v\n", issue.Labels)
	}
	fmt.Printf("  Created:     %s\n", issue.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Updated:     %s\n", issue.UpdatedAt.Format("2006-01-02 15:04:05"))
	if issue.ClosedAt != nil {
		fmt.Printf("  Closed:      %s\n", issue.ClosedAt.Format("2006-01-02 15:04:05"))
	}
}

// printMessage prints a simple message (used for non-issue results).
func printMessage(msg string, pretty bool) {
	if pretty {
		fmt.Println(msg)
		return
	}
	printJSON(map[string]string{"message": msg})
}
