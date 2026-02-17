package cli

import (
	"flag"
	"fmt"
)

func runSync(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	full := fs.Bool("full", false, "Perform a full replay sync instead of incremental")

	if err := fs.Parse(args); err != nil {
		return err
	}

	client := newClient(gf)
	repo := resolveRepo(gf)

	var err error
	if *full {
		err = client.ForceSyncFull(repo)
	} else {
		err = client.ForceSync(repo)
	}
	if err != nil {
		return err
	}

	mode := "incremental"
	if *full {
		mode = "full"
	}

	if gf.pretty {
		fmt.Printf("Sync triggered (%s).\n", mode)
	} else {
		printJSON(map[string]string{
			"status": "sync triggered",
			"mode":   mode,
		})
	}

	return nil
}
