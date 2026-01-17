package probe

import "testing"

func TestSeverityString(t *testing.T) {
	tests := []struct {
		severity Severity
		expected string
	}{
		{SeverityOK, "OK"},
		{SeverityWarning, "WARNING"},
		{SeverityCritical, "CRITICAL"},
		{Severity(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		got := tt.severity.String()
		if got != tt.expected {
			t.Errorf("Severity(%d).String() = %q, want %q", tt.severity, got, tt.expected)
		}
	}
}

func TestSeverityOrdering(t *testing.T) {
	if SeverityOK >= SeverityWarning {
		t.Error("SeverityOK should be less than SeverityWarning")
	}
	if SeverityWarning >= SeverityCritical {
		t.Error("SeverityWarning should be less than SeverityCritical")
	}
}

func TestCheckResultMaxSeverity(t *testing.T) {
	tests := []struct {
		name     string
		results  []Result
		expected Severity
	}{
		{
			name:     "empty results",
			results:  []Result{},
			expected: SeverityOK,
		},
		{
			name: "all OK",
			results: []Result{
				{Severity: SeverityOK},
				{Severity: SeverityOK},
			},
			expected: SeverityOK,
		},
		{
			name: "mixed with warning",
			results: []Result{
				{Severity: SeverityOK},
				{Severity: SeverityWarning},
				{Severity: SeverityOK},
			},
			expected: SeverityWarning,
		},
		{
			name: "mixed with critical",
			results: []Result{
				{Severity: SeverityOK},
				{Severity: SeverityWarning},
				{Severity: SeverityCritical},
			},
			expected: SeverityCritical,
		},
		{
			name: "critical first",
			results: []Result{
				{Severity: SeverityCritical},
				{Severity: SeverityOK},
			},
			expected: SeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cr := &CheckResult{Results: tt.results}
			got := cr.MaxSeverity()
			if got != tt.expected {
				t.Errorf("MaxSeverity() = %v, want %v", got, tt.expected)
			}
		})
	}
}
