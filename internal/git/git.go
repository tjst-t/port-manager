package git

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Info holds git repository information.
type Info struct {
	Project      string // e.g. "tjst-t/palmux"
	Repo         string // e.g. "palmux"
	Worktree     string // branch name, e.g. "feature-xyz"
	WorktreePath string // absolute path to worktree root
}

// Detect retrieves git information from the current directory.
func Detect() (Info, error) {
	var info Info

	project, err := detectProject()
	if err != nil {
		return info, err
	}
	info.Project = project

	parts := strings.Split(project, "/")
	info.Repo = parts[len(parts)-1]

	branch, err := detectBranch()
	if err != nil {
		return info, err
	}
	info.Worktree = branch

	toplevel, err := detectToplevel()
	if err != nil {
		return info, err
	}
	info.WorktreePath = toplevel

	return info, nil
}

// detectProject extracts org/repo from git remote origin URL.
func detectProject() (string, error) {
	out, err := runGit("remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("getting remote URL: %w (is this a git repository with a remote?)", err)
	}
	return ParseRemoteURL(out)
}

// detectBranch gets the current branch name.
func detectBranch() (string, error) {
	out, err := runGit("branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("getting current branch: %w", err)
	}
	if out == "" {
		return "", fmt.Errorf("detached HEAD state: use --worktree to specify a worktree name")
	}
	return out, nil
}

// detectToplevel gets the worktree root path.
func detectToplevel() (string, error) {
	out, err := runGit("rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("getting worktree path: %w", err)
	}
	return out, nil
}

// runGit executes a git command and returns trimmed stdout.
func runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// sshURLPattern matches SSH-style git URLs: git@host:org/repo.git
var sshURLPattern = regexp.MustCompile(`^[^@]+@[^:]+:(.+?)(?:\.git)?$`)

// ParseRemoteURL extracts org/repo from a git remote URL.
// Supports SSH (git@github.com:org/repo.git) and HTTPS (https://github.com/org/repo.git).
func ParseRemoteURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)

	// Try SSH format: git@github.com:org/repo.git
	if matches := sshURLPattern.FindStringSubmatch(rawURL); matches != nil {
		return matches[1], nil
	}

	// Try HTTPS format: https://github.com/org/repo.git
	// Remove scheme
	u := rawURL
	if idx := strings.Index(u, "://"); idx >= 0 {
		u = u[idx+3:]
	}

	// Remove .git suffix
	u = strings.TrimSuffix(u, ".git")

	// Split by / and take the path after the host
	parts := strings.SplitN(u, "/", 2)
	if len(parts) < 2 || parts[1] == "" {
		return "", fmt.Errorf("cannot parse remote URL: %s", rawURL)
	}

	path := parts[1]
	// Remove leading slash if present
	path = strings.TrimPrefix(path, "/")

	if path == "" {
		return "", fmt.Errorf("cannot parse remote URL: %s", rawURL)
	}

	return path, nil
}

// dnsInvalidChars matches characters not valid in DNS labels.
var dnsInvalidChars = regexp.MustCompile(`[^a-zA-Z0-9-]`)

// leadingTrailingHyphens matches leading/trailing hyphens.
var leadingTrailingHyphens = regexp.MustCompile(`^-+|-+$`)

// sanitizeDNSLabel replaces DNS-invalid characters and trims hyphens.
func sanitizeDNSLabel(s string) string {
	s = dnsInvalidChars.ReplaceAllString(s, "-")
	s = leadingTrailingHyphens.ReplaceAllString(s, "")
	return s
}

// GenerateHostname generates a hostname from service components and a pattern.
// The pattern uses {name}, {worktree}, {repo} placeholders.
// The result is appended with the domain suffix.
func GenerateHostname(name, worktree, repo, pattern, domainSuffix string) (string, error) {
	host := pattern
	host = strings.ReplaceAll(host, "{name}", sanitizeDNSLabel(name))
	host = strings.ReplaceAll(host, "{worktree}", sanitizeDNSLabel(worktree))
	host = strings.ReplaceAll(host, "{repo}", sanitizeDNSLabel(repo))

	// Clean up multiple consecutive hyphens
	for strings.Contains(host, "---") {
		host = strings.ReplaceAll(host, "---", "--")
	}

	fqdn := host + "." + domainSuffix

	if len(host) > 63 {
		return "", fmt.Errorf("hostname label exceeds 63 characters: %s (%d chars)", host, len(host))
	}

	return fqdn, nil
}
