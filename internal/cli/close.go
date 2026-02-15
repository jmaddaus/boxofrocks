package cli

import (
	"fmt"
	"strconv"
)

func runClose(args []string, gf globalFlags) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bor close <id>")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue id %q: %w", args[0], err)
	}

	client := newClient(gf)

	fields := map[string]interface{}{
		"status": "closed",
	}
	issue, err := client.UpdateIssue(id, fields)
	if err != nil {
		return fmt.Errorf("close issue: %w", err)
	}

	printIssue(issue, gf.pretty)
	return nil
}
