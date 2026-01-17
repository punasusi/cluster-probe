package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/punasusi/cluster-probe/pkg/probe"
	"github.com/punasusi/cluster-probe/pkg/probe/storage"
)

func TestNewWriter(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatText, true)

	if w.format != FormatText {
		t.Error("format should be text")
	}
	if !w.verbose {
		t.Error("verbose should be true")
	}
}

func TestWriteTextDefault(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatText, false)

	results := []probe.CheckResult{
		{
			Name: "test-check",
			Tier: 1,
			Results: []probe.Result{
				{Severity: probe.SeverityCritical, Message: "critical issue"},
				{Severity: probe.SeverityOK, Message: "ok issue"},
			},
		},
	}

	if err := w.Write(results, "test-cluster"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "CLUSTER PROBE REPORT") {
		t.Error("missing report header")
	}
	if !strings.Contains(output, "test-cluster") {
		t.Error("missing cluster name")
	}
	if !strings.Contains(output, "critical issue") {
		t.Error("missing critical issue in default output")
	}
	if !strings.Contains(output, "1 critical") {
		t.Error("missing critical count")
	}
}

func TestWriteTextVerbose(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatText, true)

	results := []probe.CheckResult{
		{
			Name: "test-check",
			Tier: 1,
			Results: []probe.Result{
				{Severity: probe.SeverityOK, Message: "ok message"},
				{Severity: probe.SeverityWarning, Message: "warning message"},
			},
		},
	}

	if err := w.Write(results, "test-cluster"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "ok message") {
		t.Error("verbose output should include OK messages")
	}
	if !strings.Contains(output, "warning message") {
		t.Error("verbose output should include warning messages")
	}
	if !strings.Contains(output, "Critical Checks") {
		t.Error("verbose output should show tier headers")
	}
}

func TestWriteJSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatJSON, false)

	results := []probe.CheckResult{
		{
			Name: "test-check",
			Tier: 1,
			Results: []probe.Result{
				{Severity: probe.SeverityCritical, Message: "critical"},
				{Severity: probe.SeverityOK, Message: "ok"},
			},
		},
	}

	if err := w.Write(results, "test-cluster"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var report Report
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	if report.Cluster != "test-cluster" {
		t.Errorf("cluster mismatch: got %q", report.Cluster)
	}
	if report.Summary.Total != 1 {
		t.Errorf("expected 1 total check, got %d", report.Summary.Total)
	}
	if report.Summary.Critical != 1 {
		t.Errorf("expected 1 critical, got %d", report.Summary.Critical)
	}
	if len(report.CheckResults) != 1 {
		t.Errorf("expected 1 check result, got %d", len(report.CheckResults))
	}
	if len(report.CheckResults[0].Results) != 2 {
		t.Errorf("JSON should include all results including OK, got %d", len(report.CheckResults[0].Results))
	}
}

func TestWriteWithDiff(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatText, false)

	diff := &storage.ScanDiff{
		HasPrevious:  true,
		PreviousTime: time.Now().Add(-time.Hour),
		NewIssues: []storage.StoredIssue{
			{CheckName: "check1", Severity: "WARNING", Message: "new issue"},
		},
		ResolvedIssues: []storage.StoredIssue{
			{CheckName: "check2", Severity: "CRITICAL", Message: "resolved issue"},
		},
		SummaryChange: storage.SummaryDiff{
			CriticalDelta: -1,
			WarningDelta:  1,
		},
	}
	w.SetDiff(diff)

	results := []probe.CheckResult{}
	if err := w.Write(results, "test-cluster"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	output := buf.String()

	if !strings.Contains(output, "New Issues") {
		t.Error("missing new issues section")
	}
	if !strings.Contains(output, "new issue") {
		t.Error("missing new issue message")
	}
	if !strings.Contains(output, "Resolved Issues") {
		t.Error("missing resolved issues section")
	}
	if !strings.Contains(output, "resolved issue") {
		t.Error("missing resolved issue message")
	}
	if !strings.Contains(output, "since last scan") {
		t.Error("missing delta in summary")
	}
}

func TestWriteJSONWithDiff(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatJSON, false)

	diff := &storage.ScanDiff{
		HasPrevious:  true,
		PreviousTime: time.Now().Add(-time.Hour),
		NewIssues: []storage.StoredIssue{
			{CheckName: "check1", Severity: "WARNING", Message: "new issue"},
		},
		SummaryChange: storage.SummaryDiff{CriticalDelta: 1},
	}
	w.SetDiff(diff)

	results := []probe.CheckResult{}
	if err := w.Write(results, "test-cluster"); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	var report Report
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if report.Diff == nil {
		t.Fatal("diff should be included in JSON")
	}
	if len(report.Diff.NewIssues) != 1 {
		t.Errorf("expected 1 new issue, got %d", len(report.Diff.NewIssues))
	}
	if report.Diff.CriticalDelta != 1 {
		t.Errorf("expected critical delta 1, got %d", report.Diff.CriticalDelta)
	}
}

func TestResultsSortedByTier(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatJSON, false)

	results := []probe.CheckResult{
		{Name: "check-tier3", Tier: 3, Results: []probe.Result{{Severity: probe.SeverityOK}}},
		{Name: "check-tier1", Tier: 1, Results: []probe.Result{{Severity: probe.SeverityOK}}},
		{Name: "check-tier2", Tier: 2, Results: []probe.Result{{Severity: probe.SeverityOK}}},
	}

	if err := w.Write(results, "test"); err != nil {
		t.Fatal(err)
	}

	var report Report
	json.Unmarshal(buf.Bytes(), &report)

	if report.CheckResults[0].Tier != 1 {
		t.Error("first check should be tier 1")
	}
	if report.CheckResults[1].Tier != 2 {
		t.Error("second check should be tier 2")
	}
	if report.CheckResults[2].Tier != 3 {
		t.Error("third check should be tier 3")
	}
}

func TestSummaryCalculation(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf, FormatJSON, false)

	results := []probe.CheckResult{
		{Name: "check1", Tier: 1, Results: []probe.Result{{Severity: probe.SeverityCritical}}},
		{Name: "check2", Tier: 1, Results: []probe.Result{{Severity: probe.SeverityWarning}}},
		{Name: "check3", Tier: 1, Results: []probe.Result{{Severity: probe.SeverityOK}}},
		{Name: "check4", Tier: 1, Results: []probe.Result{{Severity: probe.SeverityOK}}},
	}

	w.Write(results, "test")

	var report Report
	json.Unmarshal(buf.Bytes(), &report)

	if report.Summary.Total != 4 {
		t.Errorf("expected total 4, got %d", report.Summary.Total)
	}
	if report.Summary.Critical != 1 {
		t.Errorf("expected critical 1, got %d", report.Summary.Critical)
	}
	if report.Summary.Warning != 1 {
		t.Errorf("expected warning 1, got %d", report.Summary.Warning)
	}
	if report.Summary.OK != 2 {
		t.Errorf("expected ok 2, got %d", report.Summary.OK)
	}
}

func TestSeverityIcon(t *testing.T) {
	tests := []struct {
		severity string
		expected string
	}{
		{"OK", "✓"},
		{"WARNING", "⚠"},
		{"CRITICAL", "✗"},
		{"UNKNOWN", "?"},
	}

	for _, tt := range tests {
		got := severityIcon(tt.severity)
		if got != tt.expected {
			t.Errorf("severityIcon(%q) = %q, want %q", tt.severity, got, tt.expected)
		}
	}
}
