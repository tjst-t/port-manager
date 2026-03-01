package git

import (
	"strings"
	"testing"
)

func TestParseRemoteURL_SSH(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"git@github.com:tjst-t/palmux.git", "tjst-t/palmux"},
		{"git@github.com:tjst-t/palmux", "tjst-t/palmux"},
		{"git@gitlab.com:org/repo.git", "org/repo"},
		{"git@bitbucket.org:team/project.git", "team/project"},
		{"git@github.com:org/sub/repo.git", "org/sub/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result, err := ParseRemoteURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseRemoteURL_HTTPS(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://github.com/tjst-t/palmux.git", "tjst-t/palmux"},
		{"https://github.com/tjst-t/palmux", "tjst-t/palmux"},
		{"https://gitlab.com/org/repo.git", "org/repo"},
		{"http://github.com/org/repo.git", "org/repo"},
		{"https://github.com/org/sub/repo.git", "org/sub/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			result, err := ParseRemoteURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestParseRemoteURL_Invalid(t *testing.T) {
	tests := []string{
		"",
		"not-a-url",
		"https://github.com/",
		"https://github.com",
	}

	for _, url := range tests {
		t.Run(url, func(t *testing.T) {
			_, err := ParseRemoteURL(url)
			if err == nil {
				t.Error("expected error for invalid URL")
			}
		})
	}
}

func TestGenerateHostname(t *testing.T) {
	tests := []struct {
		name      string
		worktree  string
		repo      string
		pattern   string
		suffix    string
		expected  string
		expectErr bool
	}{
		{
			name:     "api",
			worktree: "feature-xyz",
			repo:     "palmux",
			pattern:  "{name}--{worktree}--{repo}",
			suffix:   "cdev.vm.tjstkm.net",
			expected: "api--feature-xyz--palmux.cdev.vm.tjstkm.net",
		},
		{
			name:     "default",
			worktree: "main",
			repo:     "palmux",
			pattern:  "{name}--{worktree}--{repo}",
			suffix:   "cdev.vm.tjstkm.net",
			expected: "default--main--palmux.cdev.vm.tjstkm.net",
		},
		{
			name:     "api",
			worktree: "feature/slash",
			repo:     "palmux",
			pattern:  "{name}--{worktree}--{repo}",
			suffix:   "cdev.vm.tjstkm.net",
			expected: "api--feature-slash--palmux.cdev.vm.tjstkm.net",
		},
		{
			name:     "api",
			worktree: "main",
			repo:     "my_repo",
			pattern:  "{name}--{worktree}--{repo}",
			suffix:   "cdev.vm.tjstkm.net",
			expected: "api--main--my-repo.cdev.vm.tjstkm.net",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result, err := GenerateHostname(tt.name, tt.worktree, tt.repo, tt.pattern, tt.suffix)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGenerateHostname_TooLong(t *testing.T) {
	longName := strings.Repeat("a", 60)
	_, err := GenerateHostname(longName, "main", "repo", "{name}--{worktree}--{repo}", "example.com")
	if err == nil {
		t.Error("expected error for hostname exceeding 63 chars")
	}
}

func TestSanitizeDNSLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with-hyphen", "with-hyphen"},
		{"with_underscore", "with-underscore"},
		{"with/slash", "with-slash"},
		{"feature/branch", "feature-branch"},
		{"-leading", "leading"},
		{"trailing-", "trailing"},
		{"-both-", "both"},
		{"MixedCase", "MixedCase"},
		{"dots.are.replaced", "dots-are-replaced"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeDNSLabel(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
