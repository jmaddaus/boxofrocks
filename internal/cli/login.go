package cli

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jmaddaus/boxofrocks/internal/github"
)

func runLogin(args []string, gf globalFlags) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	tokenFlag := fs.String("token", "", "GitHub personal access token")
	status := fs.Bool("status", false, "Show current auth status")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *status {
		return runLoginStatus()
	}

	token := *tokenFlag

	// If no --token flag, read from stdin.
	if token == "" {
		fi, _ := os.Stdin.Stat()
		if fi.Mode()&os.ModeCharDevice != 0 {
			// Interactive TTY — prompt the user.
			fmt.Print("Enter GitHub token: ")
		}
		scanner := bufio.NewScanner(os.Stdin)
		if scanner.Scan() {
			token = strings.TrimSpace(scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read token: %w", err)
		}
	}

	if token == "" {
		return fmt.Errorf("no token provided; use --token or pipe via stdin")
	}

	// Validate the token against GitHub API.
	username, err := github.ValidateToken(token)
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	// Save the token.
	if err := github.SaveToken(token); err != nil {
		return fmt.Errorf("save token: %w", err)
	}

	fmt.Printf("Authenticated as @%s. Token saved.\n", username)
	return nil
}

func runLogout(args []string, gf globalFlags) error {
	if err := github.RemoveToken(); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	fmt.Println("Token removed.")
	return nil
}

func runLoginStatus() error {
	methods, err := github.ResolveTokenWithMethod()
	if err != nil {
		fmt.Println("No GitHub token found via any method.")
		fmt.Println()
		fmt.Println("To authenticate, use one of:")
		fmt.Println("  bor login              Enter a token interactively")
		fmt.Println("  bor login --token TOK  Provide a token directly")
		fmt.Println("  gh auth login          Use GitHub CLI")
		fmt.Println("  export GITHUB_TOKEN=.. Set environment variable")
		return nil
	}

	fmt.Printf("Found %d auth method(s):\n", len(methods))
	for i, m := range methods {
		prefix := "  "
		if i == 0 {
			prefix = "* " // active method
		}
		// Mask the token for display.
		masked := maskToken(m.Token)
		fmt.Printf("%s%-25s %s\n", prefix, m.Name, masked)
	}

	// Validate the active (first) token.
	fmt.Println()
	username, err := github.ValidateToken(methods[0].Token)
	if err != nil {
		fmt.Printf("Active token validation failed: %v\n", err)
	} else {
		fmt.Printf("Authenticated as @%s (via %s)\n", username, methods[0].Name)
	}

	return nil
}

// maskToken shows the first 4 and last 4 characters of a token.
func maskToken(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}
