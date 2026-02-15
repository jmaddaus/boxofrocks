package cli

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// detectRepo attempts to determine the repository owner/name from the
// current directory's git remote "origin" URL.
// Supported formats:
//   - https://github.com/owner/name.git
//   - https://github.com/owner/name
//   - git@github.com:owner/name.git
//   - git@github.com:owner/name
func detectRepo() (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("not inside a git repo or no 'origin' remote: %w", err)
	}
	url := strings.TrimSpace(string(out))
	return parseGitRemoteURL(url)
}

var (
	httpsRe = regexp.MustCompile(`^https?://[^/]+/([^/]+)/([^/]+?)(?:\.git)?$`)
	sshRe   = regexp.MustCompile(`^git@[^:]+:([^/]+)/([^/]+?)(?:\.git)?$`)
)

// parseGitRemoteURL extracts "owner/name" from a git remote URL.
func parseGitRemoteURL(url string) (string, error) {
	if m := httpsRe.FindStringSubmatch(url); m != nil {
		return m[1] + "/" + m[2], nil
	}
	if m := sshRe.FindStringSubmatch(url); m != nil {
		return m[1] + "/" + m[2], nil
	}
	return "", fmt.Errorf("unable to parse git remote URL: %s", url)
}
