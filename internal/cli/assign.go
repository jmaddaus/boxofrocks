package cli

import (
	"fmt"
	"strconv"
)

func runAssign(args []string, gf globalFlags) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: bor assign <id> <owner>")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue id %q: %w", args[0], err)
	}
	owner := args[1]

	client := newClient(gf)

	issue, err := client.AssignIssue(id, owner)
	if err != nil {
		return fmt.Errorf("assign issue: %w", err)
	}

	printIssue(issue, gf.pretty)
	return nil
}
