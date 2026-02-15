package cli

import "fmt"

func runNext(args []string, gf globalFlags) error {
	client := newClient(gf)
	repo := resolveRepo(gf)

	issue, err := client.NextIssue(repo)
	if err != nil {
		return fmt.Errorf("next issue: %w", err)
	}

	printIssue(issue, gf.pretty)
	return nil
}
