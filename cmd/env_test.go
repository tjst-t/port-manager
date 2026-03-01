package cmd

import "testing"

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
