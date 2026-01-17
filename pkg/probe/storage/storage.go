package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	ProbeDir	= ".probe"
	LastScanFile	= "last-scan.json"
	ConfigFile	= "config.yaml"
)

type ScanRecord struct {
	Timestamp	time.Time	`json:"timestamp"`
	Cluster		string		`json:"cluster"`
	Summary		ScanSummary	`json:"summary"`
	Issues		[]StoredIssue	`json:"issues"`
}

type ScanSummary struct {
	Total		int	`json:"total"`
	Critical	int	`json:"critical"`
	Warning		int	`json:"warning"`
	OK		int	`json:"ok"`
}

type StoredIssue struct {
	CheckName	string	`json:"check"`
	Severity	string	`json:"severity"`
	Message		string	`json:"message"`
	Fingerprint	string	`json:"fingerprint"`	// Unique identifier for comparison
}

type ScanDiff struct {
	HasPrevious	bool		`json:"has_previous"`
	PreviousTime	time.Time	`json:"previous_time,omitempty"`
	NewIssues	[]StoredIssue	`json:"new_issues,omitempty"`
	ResolvedIssues	[]StoredIssue	`json:"resolved_issues,omitempty"`
	SummaryChange	SummaryDiff	`json:"summary_change,omitempty"`
}

type SummaryDiff struct {
	CriticalDelta	int	`json:"critical_delta"`
	WarningDelta	int	`json:"warning_delta"`
	OKDelta		int	`json:"ok_delta"`
}

type Storage struct {
	baseDir string
}

func NewStorage(baseDir string) *Storage {
	if baseDir == "" {
		baseDir = "."
	}
	return &Storage{baseDir: baseDir}
}

func (s *Storage) ProbeDirPath() string {
	return filepath.Join(s.baseDir, ProbeDir)
}

func (s *Storage) EnsureProbeDir() error {
	dir := s.ProbeDirPath()
	return os.MkdirAll(dir, 0755)
}

func (s *Storage) LastScanPath() string {
	return filepath.Join(s.ProbeDirPath(), LastScanFile)
}

func (s *Storage) ConfigPath() string {
	return filepath.Join(s.ProbeDirPath(), ConfigFile)
}

func (s *Storage) LoadLastScan() (*ScanRecord, error) {
	path := s.LastScanPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read last scan: %w", err)
	}

	var record ScanRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to parse last scan: %w", err)
	}

	return &record, nil
}

func (s *Storage) SaveScan(record *ScanRecord) error {
	if err := s.EnsureProbeDir(); err != nil {
		return fmt.Errorf("failed to create .probe directory: %w", err)
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal scan record: %w", err)
	}

	path := s.LastScanPath()
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write scan record: %w", err)
	}

	return nil
}

func ComputeDiff(current, previous *ScanRecord) *ScanDiff {
	diff := &ScanDiff{
		HasPrevious: previous != nil,
	}

	if previous == nil {
		return diff
	}

	diff.PreviousTime = previous.Timestamp

	prevIssues := make(map[string]StoredIssue)
	for _, issue := range previous.Issues {
		prevIssues[issue.Fingerprint] = issue
	}

	currIssues := make(map[string]StoredIssue)
	for _, issue := range current.Issues {
		currIssues[issue.Fingerprint] = issue
	}

	for fp, issue := range currIssues {
		if _, exists := prevIssues[fp]; !exists {
			diff.NewIssues = append(diff.NewIssues, issue)
		}
	}

	for fp, issue := range prevIssues {
		if _, exists := currIssues[fp]; !exists {
			diff.ResolvedIssues = append(diff.ResolvedIssues, issue)
		}
	}

	diff.SummaryChange = SummaryDiff{
		CriticalDelta:	current.Summary.Critical - previous.Summary.Critical,
		WarningDelta:	current.Summary.Warning - previous.Summary.Warning,
		OKDelta:	current.Summary.OK - previous.Summary.OK,
	}

	return diff
}

func GenerateFingerprint(checkName, severity, message string) string {

	return fmt.Sprintf("%s|%s|%s", checkName, severity, message)
}
