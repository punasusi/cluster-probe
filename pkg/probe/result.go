package probe

type Severity int

const (
	SeverityOK	Severity	= iota
	SeverityWarning
	SeverityCritical
)

func (s Severity) String() string {
	switch s {
	case SeverityOK:
		return "OK"
	case SeverityWarning:
		return "WARNING"
	case SeverityCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

type Result struct {
	CheckName	string
	Severity	Severity
	Message		string
	Details		[]string
	Remediation	string
}

type CheckResult struct {
	Name	string
	Tier	int
	Results	[]Result
}

func (cr *CheckResult) MaxSeverity() Severity {
	max := SeverityOK
	for _, r := range cr.Results {
		if r.Severity > max {
			max = r.Severity
		}
	}
	return max
}
