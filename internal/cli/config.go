package cli

import (
	"fmt"
	"strings"
)

func runConfig(args []string, gf globalFlags) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bor config <setting> <value>\n\nSettings:\n  trusted-authors-only true|false   Enable/disable trusted author filtering")
	}

	setting := args[0]
	switch setting {
	case "trusted-authors-only":
		return runConfigTrustedAuthors(args[1:], gf)
	default:
		return fmt.Errorf("unknown config setting: %s", setting)
	}
}

func runConfigTrustedAuthors(args []string, gf globalFlags) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: bor config trusted-authors-only <true|false>")
	}

	val := strings.ToLower(args[0])
	var enabled bool
	switch val {
	case "true", "1", "on", "yes":
		enabled = true
	case "false", "0", "off", "no":
		enabled = false
	default:
		return fmt.Errorf("invalid value %q: use true or false", args[0])
	}

	client := newClient(gf)
	repo := resolveRepo(gf)

	fields := map[string]interface{}{
		"trusted_authors_only": enabled,
	}
	updated, err := client.UpdateRepo(repo, fields)
	if err != nil {
		return err
	}

	fmt.Printf("trusted_authors_only = %v (repo: %s/%s)\n", updated.TrustedAuthorsOnly, updated.Owner, updated.Name)
	return nil
}
