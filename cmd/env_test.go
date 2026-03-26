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

func TestParseRange(t *testing.T) {
	tests := []struct {
		entry     string
		wantName  string
		wantCount int
		wantErr   bool
	}{
		{"libvirt-hosts=20", "libvirt-hosts", 20, false},
		{"ovn-clusters=5", "ovn-clusters", 5, false},
		{"single=1", "single", 1, false},
		{"bad-format", "", 0, true},
		{"=5", "", 0, true},
		{"name=0", "", 0, true},
		{"name=-1", "", 0, true},
		{"name=abc", "", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			name, count, err := parseRange(tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tt.wantName {
				t.Errorf("name: expected %q, got %q", tt.wantName, name)
			}
			if count != tt.wantCount {
				t.Errorf("count: expected %d, got %d", tt.wantCount, count)
			}
		})
	}
}

func TestNameToEnvVarRange(t *testing.T) {
	tests := []struct {
		name      string
		wantStart string
		wantEnd   string
	}{
		{"libvirt-hosts", "LIBVIRT_HOSTS_PORT_START", "LIBVIRT_HOSTS_PORT_END"},
		{"ovn-clusters", "OVN_CLUSTERS_PORT_START", "OVN_CLUSTERS_PORT_END"},
		{"simple", "SIMPLE_PORT_START", "SIMPLE_PORT_END"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := nameToEnvVarRange(tt.name)
			if start != tt.wantStart {
				t.Errorf("start: expected %q, got %q", tt.wantStart, start)
			}
			if end != tt.wantEnd {
				t.Errorf("end: expected %q, got %q", tt.wantEnd, end)
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
