package cmd

import "testing"

func TestParseName(t *testing.T) {
	tests := []struct {
		entry      string
		wantName   string
		wantExpose bool
	}{
		{"api", "api", false},
		{"dashboard:expose", "dashboard", true},
		{"my-service", "my-service", false},
		{"my-service:expose", "my-service", true},
		{"expose", "expose", false},
	}

	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			name, expose := parseName(tt.entry)
			if name != tt.wantName {
				t.Errorf("name: expected %q, got %q", tt.wantName, name)
			}
			if expose != tt.wantExpose {
				t.Errorf("expose: expected %v, got %v", tt.wantExpose, expose)
			}
		})
	}
}

func TestNameToEnvVar(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"api", "API_PORT"},
		{"db", "DB_PORT"},
		{"my-service", "MY_SERVICE_PORT"},
		{"frontend", "FRONTEND_PORT"},
		{"My-App", "MY_APP_PORT"},
		{"default", "DEFAULT_PORT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := nameToEnvVar(tt.name)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
