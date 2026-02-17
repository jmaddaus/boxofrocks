package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

func runRepos(args []string, gf globalFlags) error {
	client := newClient(gf)

	repos, err := client.ListRepos()
	if err != nil {
		return err
	}

	if !gf.pretty {
		printJSON(repos)
		return nil
	}

	if len(repos) == 0 {
		fmt.Println("No repos registered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REPO\tTRUSTED\tPATHS\tSOCKET\tQUEUE")
	for _, repo := range repos {
		paths := make([]string, 0, len(repo.LocalPaths))
		hasSocket := false
		hasQueue := false
		for _, lp := range repo.LocalPaths {
			paths = append(paths, lp.LocalPath)
			if lp.SocketEnabled {
				hasSocket = true
			}
			if lp.QueueEnabled {
				hasQueue = true
			}
		}
		pathStr := "-"
		if len(paths) > 0 {
			pathStr = strings.Join(paths, ", ")
		}
		fmt.Fprintf(w, "%s\t%v\t%s\t%v\t%v\n",
			repo.FullName(),
			repo.TrustedAuthorsOnly,
			pathStr,
			hasSocket,
			hasQueue,
		)
	}
	w.Flush()
	return nil
}
