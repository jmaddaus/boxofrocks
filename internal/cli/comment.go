package cli

import (
	"fmt"
	"strconv"
	"strings"
)

func runComment(args []string, gf globalFlags) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: bor comment <id> <message>")
	}

	id, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid issue id %q: %w", args[0], err)
	}

	comment := strings.Join(args[1:], " ")
	if comment == "" {
		return fmt.Errorf("comment message is required")
	}

	client := newClient(gf)

	issue, err := client.CommentIssue(id, comment)
	if err != nil {
		return fmt.Errorf("comment issue: %w", err)
	}

	printIssue(issue, gf.pretty)
	return nil
}
