package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewStorage(t *testing.T) {
	s := NewStorage("")
	if s.baseDir != "." {
		t.Errorf("expected baseDir '.', got %q", s.baseDir)
	}

	s = NewStorage("/custom/path")
	if s.baseDir != "/custom/path" {
		t.Errorf("expected baseDir '/custom/path', got %q", s.baseDir)
	}
}

func TestStoragePaths(t *testing.T) {
	s := NewStorage("/base")

	if s.ProbeDirPath() != "/base/.probe" {
		t.Errorf("unexpected probe dir path: %s", s.ProbeDirPath())
	}
	if s.LastScanPath() != "/base/.probe/last-scan.json" {
		t.Errorf("unexpected last scan path: %s", s.LastScanPath())
	}
	if s.ConfigPath() != "/base/.probe/config.yaml" {
		t.Errorf("unexpected config path: %s", s.ConfigPath())
	}
}

func TestEnsureProbeDir(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStorage(tmpDir)

	if err := s.EnsureProbeDir(); err != nil {
		t.Fatalf("EnsureProbeDir failed: %v", err)
	}

	probeDir := s.ProbeDirPath()
	info, err := os.Stat(probeDir)
	if err != nil {
		t.Fatalf("probe dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("probe dir should be a directory")
	}
}

func TestLoadLastScanNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStorage(tmpDir)

	record, err := s.LoadLastScan()
	if err != nil {
		t.Errorf("unexpected error for nonexistent scan: %v", err)
	}
	if record != nil {
		t.Error("expected nil record for nonexistent scan")
	}
}

func TestSaveAndLoadScan(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStorage(tmpDir)

	record := &ScanRecord{
		Timestamp: time.Now().UTC(),
		Cluster:   "test-cluster",
		Summary: ScanSummary{
			Total:    10,
			Critical: 1,
			Warning:  2,
			OK:       7,
		},
		Issues: []StoredIssue{
			{
				CheckName:   "test-check",
				Severity:    "WARNING",
				Message:     "test message",
				Fingerprint: "test|WARNING|test message",
			},
		},
	}

	if err := s.SaveScan(record); err != nil {
		t.Fatalf("SaveScan failed: %v", err)
	}

	loaded, err := s.LoadLastScan()
	if err != nil {
		t.Fatalf("LoadLastScan failed: %v", err)
	}

	if loaded.Cluster != record.Cluster {
		t.Errorf("cluster mismatch: got %q, want %q", loaded.Cluster, record.Cluster)
	}
	if loaded.Summary.Critical != record.Summary.Critical {
		t.Errorf("critical count mismatch: got %d, want %d", loaded.Summary.Critical, record.Summary.Critical)
	}
	if len(loaded.Issues) != len(record.Issues) {
		t.Errorf("issues count mismatch: got %d, want %d", len(loaded.Issues), len(record.Issues))
	}
}

func TestLoadLastScanInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStorage(tmpDir)

	if err := s.EnsureProbeDir(); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(s.LastScanPath(), []byte("invalid json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := s.LoadLastScan()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestComputeDiffNoPrevious(t *testing.T) {
	current := &ScanRecord{
		Summary: ScanSummary{Critical: 1},
		Issues: []StoredIssue{
			{Fingerprint: "a"},
		},
	}

	diff := ComputeDiff(current, nil)

	if diff.HasPrevious {
		t.Error("HasPrevious should be false")
	}
	if len(diff.NewIssues) != 0 {
		t.Error("NewIssues should be empty when no previous")
	}
}

func TestComputeDiffWithPrevious(t *testing.T) {
	previous := &ScanRecord{
		Timestamp: time.Now().Add(-time.Hour),
		Summary:   ScanSummary{Critical: 2, Warning: 3, OK: 5},
		Issues: []StoredIssue{
			{Fingerprint: "a", Message: "issue a"},
			{Fingerprint: "b", Message: "issue b"},
		},
	}

	current := &ScanRecord{
		Timestamp: time.Now(),
		Summary:   ScanSummary{Critical: 1, Warning: 4, OK: 5},
		Issues: []StoredIssue{
			{Fingerprint: "b", Message: "issue b"},
			{Fingerprint: "c", Message: "issue c"},
		},
	}

	diff := ComputeDiff(current, previous)

	if !diff.HasPrevious {
		t.Error("HasPrevious should be true")
	}

	if len(diff.NewIssues) != 1 {
		t.Errorf("expected 1 new issue, got %d", len(diff.NewIssues))
	}
	if diff.NewIssues[0].Fingerprint != "c" {
		t.Errorf("unexpected new issue: %v", diff.NewIssues[0])
	}

	if len(diff.ResolvedIssues) != 1 {
		t.Errorf("expected 1 resolved issue, got %d", len(diff.ResolvedIssues))
	}
	if diff.ResolvedIssues[0].Fingerprint != "a" {
		t.Errorf("unexpected resolved issue: %v", diff.ResolvedIssues[0])
	}

	if diff.SummaryChange.CriticalDelta != -1 {
		t.Errorf("expected critical delta -1, got %d", diff.SummaryChange.CriticalDelta)
	}
	if diff.SummaryChange.WarningDelta != 1 {
		t.Errorf("expected warning delta 1, got %d", diff.SummaryChange.WarningDelta)
	}
}

func TestGenerateFingerprint(t *testing.T) {
	fp := GenerateFingerprint("check", "WARNING", "message")
	expected := "check|WARNING|message"
	if fp != expected {
		t.Errorf("expected %q, got %q", expected, fp)
	}
}

func TestSaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "path")
	s := NewStorage(nestedPath)

	record := &ScanRecord{Cluster: "test"}
	if err := s.SaveScan(record); err != nil {
		t.Fatalf("SaveScan should create nested directories: %v", err)
	}

	if _, err := os.Stat(s.ProbeDirPath()); err != nil {
		t.Error("probe dir should exist after save")
	}
}
