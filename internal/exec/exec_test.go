package exec

import (
	"testing"
	"time"
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

func TestStart_CommandNotFound(t *testing.T) {
	_, err := Start("nonexistent-command-xyz", []string{}, 8080)
	if err == nil {
		t.Fatal("expected error for nonexistent command")
	}
}

func TestWaitStartup_ProcessDiesImmediately(t *testing.T) {
	// "false" exits with code 1 immediately
	runner, err := Start("false", []string{}, 8080)
	if err != nil {
		t.Fatalf("unexpected Start error: %v", err)
	}

	alive, waitErr := runner.WaitStartup(2 * time.Second)
	if alive {
		t.Error("expected alive=false for immediately dying process")
	}
	if waitErr == nil {
		t.Error("expected non-nil error for exit code 1")
	}
}

func TestWaitStartup_ProcessExitsSuccessfully(t *testing.T) {
	// "true" exits with code 0 immediately
	runner, err := Start("true", []string{}, 8080)
	if err != nil {
		t.Fatalf("unexpected Start error: %v", err)
	}

	alive, waitErr := runner.WaitStartup(2 * time.Second)
	if alive {
		t.Error("expected alive=false for quickly exiting process")
	}
	if waitErr != nil {
		t.Errorf("expected nil error for exit code 0, got: %v", waitErr)
	}
}

func TestWaitStartup_ProcessStaysAlive(t *testing.T) {
	// "sleep 30" stays alive well beyond the startup timeout
	runner, err := Start("sleep", []string{"30"}, 8080)
	if err != nil {
		t.Fatalf("unexpected Start error: %v", err)
	}

	alive, waitErr := runner.WaitStartup(100 * time.Millisecond)
	if !alive {
		t.Error("expected alive=true for long-running process")
	}
	if waitErr != nil {
		t.Errorf("expected nil error, got: %v", waitErr)
	}

	// Clean up: kill the process
	if runner.cmd.Process != nil {
		runner.cmd.Process.Kill()
		runner.Wait()
	}
}
