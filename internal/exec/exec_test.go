package exec

import (
	"testing"
)

func TestReplacePortPlaceholder(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		port     int
		expected []string
	}{
		{
			name:     "single placeholder",
			args:     []string{"--port", "{}"},
			port:     8234,
			expected: []string{"--port", "8234"},
		},
		{
			name:     "no placeholder",
			args:     []string{"--verbose", "start"},
			port:     8234,
			expected: []string{"--verbose", "start"},
		},
		{
			name:     "multiple placeholders",
			args:     []string{"--port", "{}", "--addr", "localhost:{}"},
			port:     9000,
			expected: []string{"--port", "9000", "--addr", "localhost:9000"},
		},
		{
			name:     "empty args",
			args:     []string{},
			port:     8234,
			expected: []string{},
		},
		{
			name:     "placeholder in middle of string",
			args:     []string{"http://localhost:{}/api"},
			port:     8080,
			expected: []string{"http://localhost:8080/api"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ReplacePortPlaceholder(tt.args, tt.port)
			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d args, got %d", len(tt.expected), len(result))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("arg[%d]: expected %q, got %q", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestReplacePortPlaceholder_DoesNotModifyOriginal(t *testing.T) {
	original := []string{"--port", "{}"}
	_ = ReplacePortPlaceholder(original, 8080)
	if original[1] != "{}" {
		t.Error("original args were modified")
	}
}

func TestRun_Success(t *testing.T) {
	err := Run("echo", []string{"hello", "{}"}, 8080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_ExitCode(t *testing.T) {
	err := Run("sh", []string{"-c", "exit 42"}, 8080)
	if err == nil {
		t.Fatal("expected error for non-zero exit code")
	}
}

func TestRun_CommandNotFound(t *testing.T) {
	err := Run("nonexistent-command-xyz", []string{}, 8080)
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}
