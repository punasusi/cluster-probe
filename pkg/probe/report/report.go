package report

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/punasusi/cluster-probe/pkg/probe"
	"github.com/punasusi/cluster-probe/pkg/probe/storage"
)

type Format string

const (
	FormatText	Format	= "text"
	FormatJSON	Format	= "json"
)

type Report struct {
	Timestamp	time.Time	`json:"timestamp"`
	Cluster		string		`json:"cluster"`
	Summary		Summary		`json:"summary"`
	CheckResults	[]CheckOutput	`json:"checks"`
	Diff		*DiffOutput	`json:"diff,omitempty"`
}

type Summary struct {
	Total		int	`json:"total"`
	Critical	int	`json:"critical"`
	Warning		int	`json:"warning"`
	OK		int	`json:"ok"`
}

type CheckOutput struct {
	Name		string		`json:"name"`
	Tier		int		`json:"tier"`
	Severity	string		`json:"severity"`
	Results		[]ResultOutput	`json:"results"`
}

type ResultOutput struct {
	Severity	string		`json:"severity"`
	Message		string		`json:"message"`
	Details		[]string	`json:"details,omitempty"`
	Remediation	string		`json:"remediation,omitempty"`
}

type DiffOutput struct {
	PreviousTime	time.Time	`json:"previous_time"`
	NewIssues	[]IssueOutput	`json:"new_issues,omitempty"`
	ResolvedIssues	[]IssueOutput	`json:"resolved_issues,omitempty"`
	CriticalDelta	int		`json:"critical_delta"`
	WarningDelta	int		`json:"warning_delta"`
}

type IssueOutput struct {
	Check		string	`json:"check"`
	Severity	string	`json:"severity"`
	Message		string	`json:"message"`
}

type Writer struct {
	w	io.Writer
	format	Format
	verbose	bool
	diff	*storage.ScanDiff
}

func NewWriter(w io.Writer, format Format, verbose bool) *Writer {
	return &Writer{
		w:		w,
		format:		format,
		verbose:	verbose,
	}
}

func (w *Writer) SetDiff(diff *storage.ScanDiff) {
	w.diff = diff
}

func (w *Writer) Write(results []probe.CheckResult, clusterInfo string) error {
	report := w.buildReport(results, clusterInfo)

	switch w.format {
	case FormatJSON:
		return w.writeJSON(report)
	default:
		return w.writeText(report)
	}
}

func (w *Writer) buildReport(results []probe.CheckResult, clusterInfo string) *Report {

	sort.Slice(results, func(i, j int) bool {
		if results[i].Tier != results[j].Tier {
			return results[i].Tier < results[j].Tier
		}
		return results[i].Name < results[j].Name
	})

	report := &Report{
		Timestamp:	time.Now().UTC(),
		Cluster:	clusterInfo,
		CheckResults:	make([]CheckOutput, 0, len(results)),
	}

	for _, cr := range results {
		severity := cr.MaxSeverity()

		switch severity {
		case probe.SeverityCritical:
			report.Summary.Critical++
		case probe.SeverityWarning:
			report.Summary.Warning++
		case probe.SeverityOK:
			report.Summary.OK++
		}
		report.Summary.Total++

		checkOutput := CheckOutput{
			Name:		cr.Name,
			Tier:		cr.Tier,
			Severity:	severity.String(),
			Results:	make([]ResultOutput, 0),
		}

		for _, r := range cr.Results {

			if r.Severity == probe.SeverityOK && !w.verbose && w.format == FormatText {
				continue
			}

			checkOutput.Results = append(checkOutput.Results, ResultOutput{
				Severity:	r.Severity.String(),
				Message:	r.Message,
				Details:	r.Details,
				Remediation:	r.Remediation,
			})
		}

		report.CheckResults = append(report.CheckResults, checkOutput)
	}

	if w.diff != nil && w.diff.HasPrevious {
		report.Diff = &DiffOutput{
			PreviousTime:	w.diff.PreviousTime,
			CriticalDelta:	w.diff.SummaryChange.CriticalDelta,
			WarningDelta:	w.diff.SummaryChange.WarningDelta,
		}

		for _, issue := range w.diff.NewIssues {
			report.Diff.NewIssues = append(report.Diff.NewIssues, IssueOutput{
				Check:		issue.CheckName,
				Severity:	issue.Severity,
				Message:	issue.Message,
			})
		}

		for _, issue := range w.diff.ResolvedIssues {
			report.Diff.ResolvedIssues = append(report.Diff.ResolvedIssues, IssueOutput{
				Check:		issue.CheckName,
				Severity:	issue.Severity,
				Message:	issue.Message,
			})
		}
	}

	return report
}

func (w *Writer) writeJSON(report *Report) error {
	encoder := json.NewEncoder(w.w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func (w *Writer) writeText(report *Report) error {

	fmt.Fprintln(w.w)
	fmt.Fprintln(w.w, "  CLUSTER PROBE REPORT")
	fmt.Fprintln(w.w, strings.Repeat("─", 60))
	if report.Cluster != "" {
		fmt.Fprintf(w.w, "  Cluster: %s\n", report.Cluster)
	}
	fmt.Fprintf(w.w, "  Time:    %s\n", report.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Fprintln(w.w)

	if w.verbose {

		w.writeVerboseChecks(report)
	} else {

		w.writeCriticalIssues(report)
	}

	if report.Diff != nil {
		w.writeDiff(report.Diff)
	}

	summaryParts := []string{}
	if report.Summary.Critical > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("✗ %d critical", report.Summary.Critical))
	}
	if report.Summary.Warning > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("⚠ %d warning", report.Summary.Warning))
	}
	if report.Summary.OK > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("✓ %d passed", report.Summary.OK))
	}

	deltaStr := ""
	if report.Diff != nil {
		deltas := []string{}
		if report.Diff.CriticalDelta != 0 {
			sign := "+"
			if report.Diff.CriticalDelta < 0 {
				sign = ""
			}
			deltas = append(deltas, fmt.Sprintf("%s%d critical", sign, report.Diff.CriticalDelta))
		}
		if report.Diff.WarningDelta != 0 {
			sign := "+"
			if report.Diff.WarningDelta < 0 {
				sign = ""
			}
			deltas = append(deltas, fmt.Sprintf("%s%d warning", sign, report.Diff.WarningDelta))
		}
		if len(deltas) > 0 {
			deltaStr = fmt.Sprintf(" (%s since last scan)", strings.Join(deltas, ", "))
		}
	}

	fmt.Fprintf(w.w, "  Summary: %s%s\n", strings.Join(summaryParts, "  "), deltaStr)
	fmt.Fprintln(w.w)

	return nil
}

func (w *Writer) writeDiff(diff *DiffOutput) {

	if len(diff.NewIssues) > 0 {
		fmt.Fprintln(w.w, "  New Issues (since last scan):")
		for _, issue := range diff.NewIssues {
			icon := severityIcon(issue.Severity)
			fmt.Fprintf(w.w, "    %s [%s] %s\n", icon, issue.Check, issue.Message)
		}
		fmt.Fprintln(w.w)
	}

	if len(diff.ResolvedIssues) > 0 {
		fmt.Fprintln(w.w, "  Resolved Issues:")
		for _, issue := range diff.ResolvedIssues {
			fmt.Fprintf(w.w, "    ✓ [%s] %s\n", issue.Check, issue.Message)
		}
		fmt.Fprintln(w.w)
	}
}

func (w *Writer) writeCriticalIssues(report *Report) {
	hasCritical := false

	for _, check := range report.CheckResults {
		if check.Severity != "CRITICAL" {
			continue
		}

		if !hasCritical {
			fmt.Fprintln(w.w, "  Critical Issues:")
			hasCritical = true
		}

		for _, r := range check.Results {
			if r.Severity != "CRITICAL" {
				continue
			}

			fmt.Fprintf(w.w, "  ✗ [%s] %s\n", check.Name, r.Message)
			if r.Remediation != "" {
				fmt.Fprintf(w.w, "    → %s\n", r.Remediation)
			}
		}
	}

	if hasCritical {
		fmt.Fprintln(w.w)
	}
}

func (w *Writer) writeVerboseChecks(report *Report) {

	currentTier := 0
	tierNames := map[int]string{
		1:	"Critical",
		2:	"Workload",
		3:	"Resource",
		4:	"Networking",
		5:	"Security",
	}

	for _, check := range report.CheckResults {
		if check.Tier != currentTier {
			currentTier = check.Tier
			tierName := tierNames[currentTier]
			if tierName == "" {
				tierName = fmt.Sprintf("Tier %d", currentTier)
			}
			fmt.Fprintf(w.w, "  ┌─ %s Checks\n", tierName)
		}

		icon := severityIcon(check.Severity)
		fmt.Fprintf(w.w, "  │ %s %s\n", icon, check.Name)

		for _, r := range check.Results {
			rIcon := severityIcon(r.Severity)
			fmt.Fprintf(w.w, "  │   %s %s\n", rIcon, r.Message)

			for _, d := range r.Details {
				fmt.Fprintf(w.w, "  │       %s\n", d)
			}

			if r.Remediation != "" && r.Severity != "OK" {
				fmt.Fprintf(w.w, "  │       → %s\n", r.Remediation)
			}
		}
	}

	fmt.Fprintln(w.w, "  └"+strings.Repeat("─", 59))
	fmt.Fprintln(w.w)
}

func severityIcon(s string) string {
	switch s {
	case "OK":
		return "✓"
	case "WARNING":
		return "⚠"
	case "CRITICAL":
		return "✗"
	default:
		return "?"
	}
}
