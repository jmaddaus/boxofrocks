package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// TokenMethod describes how a token was resolved.
type TokenMethod struct {
	Name  string // e.g. "GITHUB_TOKEN", "token file", "gh CLI", "git credential"
	Token string
}

// TokenFilePath returns the path to the stored token file (~/.boxofrocks/token).
func TokenFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, ".boxofrocks", "token"), nil
}

// SaveToken writes a token to the token file with 0600 permissions.
func SaveToken(token string) error {
	path, err := TokenFilePath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(token)+"\n"), 0600)
}

// RemoveToken deletes the stored token file.
func RemoveToken() error {
	path, err := TokenFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove token file: %w", err)
	}
	return nil
}

// ValidateToken calls the GitHub API GET /user to verify a token is valid.
// Returns the authenticated username or an error.
func ValidateToken(token string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitHub API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return "", fmt.Errorf("token is invalid or expired (HTTP 401)")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status from GitHub API: %d", resp.StatusCode)
	}

	var user struct {
		Login string `json:"login"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return "", fmt.Errorf("decode user response: %w", err)
	}
	if user.Login == "" {
		return "", fmt.Errorf("GitHub API returned empty username")
	}
	return user.Login, nil
}

// ResolveToken attempts to resolve a GitHub API token using four methods in order:
// 1. GITHUB_TOKEN environment variable
// 2. ~/.boxofrocks/token file
// 3. gh auth token command (GitHub CLI)
// 4. git credential fill with host=github.com
// Returns the first successful token or an error if all methods fail.
func ResolveToken() (string, error) {
	// Method 1: GITHUB_TOKEN environment variable
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		return strings.TrimSpace(token), nil
	}

	// Method 2: ~/.boxofrocks/token file
	if token, err := resolveFromTokenFile(); err == nil && token != "" {
		return token, nil
	}

	// Method 3: gh auth token command
	if token, err := resolveFromGHCLI(); err == nil && token != "" {
		return token, nil
	}

	// Method 4: git credential fill
	if token, err := resolveFromGitCredential(); err == nil && token != "" {
		return token, nil
	}

	return "", fmt.Errorf("unable to resolve GitHub token; tried:\n" +
		"  1. GITHUB_TOKEN environment variable (not set)\n" +
		"  2. ~/.boxofrocks/token file (not found)\n" +
		"  3. gh auth token (failed or gh CLI not installed)\n" +
		"  4. git credential fill for github.com (failed or no credential stored)\n" +
		"Run 'bor login', set GITHUB_TOKEN, run 'gh auth login', or configure git credentials for github.com")
}

// ResolveTokenWithMethod tries each auth method and returns which one succeeded.
// Used by `bor login --status` to report the active auth source.
func ResolveTokenWithMethod() ([]TokenMethod, error) {
	var methods []TokenMethod

	// Method 1: GITHUB_TOKEN
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		methods = append(methods, TokenMethod{Name: "GITHUB_TOKEN env", Token: strings.TrimSpace(token)})
	}

	// Method 2: token file
	if token, err := resolveFromTokenFile(); err == nil && token != "" {
		methods = append(methods, TokenMethod{Name: "~/.boxofrocks/token", Token: token})
	}

	// Method 3: gh CLI
	if token, err := resolveFromGHCLI(); err == nil && token != "" {
		methods = append(methods, TokenMethod{Name: "gh auth token", Token: token})
	}

	// Method 4: git credential
	if token, err := resolveFromGitCredential(); err == nil && token != "" {
		methods = append(methods, TokenMethod{Name: "git credential fill", Token: token})
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no GitHub token found via any method")
	}
	return methods, nil
}

func resolveFromTokenFile() (string, error) {
	path, err := TokenFilePath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file is empty")
	}
	return token, nil
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
