package github

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ResolveToken attempts to resolve a GitHub API token using three methods in order:
// 1. GITHUB_TOKEN environment variable
// 2. gh auth token command (GitHub CLI)
// 3. git credential fill with host=github.com
// Returns the first successful token or an error if all methods fail.
func ResolveToken() (string, error) {
	// Method 1: GITHUB_TOKEN environment variable
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return strings.TrimSpace(token), nil
	}

	// Method 2: gh auth token command
	if token, err := resolveFromGHCLI(); err == nil && token != "" {
		return token, nil
	}

	// Method 3: git credential fill
	if token, err := resolveFromGitCredential(); err == nil && token != "" {
		return token, nil
	}

	return "", fmt.Errorf("unable to resolve GitHub token; tried:\n" +
		"  1. GITHUB_TOKEN environment variable (not set)\n" +
		"  2. gh auth token (failed or gh CLI not installed)\n" +
		"  3. git credential fill for github.com (failed or no credential stored)\n" +
		"Set GITHUB_TOKEN, run 'gh auth login', or configure git credentials for github.com")
}

func resolveFromGHCLI() (string, error) {
	cmd := exec.Command("gh", "auth", "token")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("gh auth token returned empty output")
	}
	return token, nil
}

func resolveFromGitCredential() (string, error) {
	cmd := exec.Command("git", "credential", "fill")
	cmd.Stdin = strings.NewReader("host=github.com\nprotocol=https\n\n")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "password=") {
			token := strings.TrimPrefix(line, "password=")
			if token != "" {
				return token, nil
			}
		}
	}
	return "", fmt.Errorf("no password found in git credential output")
}
