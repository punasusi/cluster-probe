package probe

import (
	"context"
	"errors"
	"testing"

	"github.com/punasusi/cluster-probe/pkg/probe/config"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type mockCheck struct {
	name    string
	tier    int
	result  *CheckResult
	err     error
	called  bool
	cfgUsed *config.Config
}

func (m *mockCheck) Name() string { return m.name }
func (m *mockCheck) Tier() int    { return m.tier }
func (m *mockCheck) Run(ctx context.Context, client kubernetes.Interface) (*CheckResult, error) {
	m.called = true
	return m.result, m.err
}
func (m *mockCheck) Configure(cfg *config.Config) {
	m.cfgUsed = cfg
}

func TestNewEngine(t *testing.T) {
	engine := NewEngine(true)
	if engine == nil {
		t.Fatal("NewEngine returned nil")
	}
	if !engine.verbose {
		t.Error("verbose should be true")
	}
}

func TestEngineRegister(t *testing.T) {
	engine := NewEngine(false)
	check := &mockCheck{name: "test-check", tier: 1}
	engine.Register(check)

	if len(engine.checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(engine.checks))
	}
}

func TestEngineRunEmpty(t *testing.T) {
	engine := NewEngine(false)
	results, err := engine.Run(context.Background(), fake.NewSimpleClientset())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestEngineRunSuccess(t *testing.T) {
	engine := NewEngine(false)
	check := &mockCheck{
		name: "test-check",
		tier: 1,
		result: &CheckResult{
			Name: "test-check",
			Tier: 1,
			Results: []Result{
				{Severity: SeverityOK, Message: "all good"},
			},
		},
	}
	engine.Register(check)

	results, err := engine.Run(context.Background(), fake.NewSimpleClientset())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if !check.called {
		t.Error("check was not called")
	}
}

func TestEngineRunError(t *testing.T) {
	engine := NewEngine(false)
	check := &mockCheck{
		name: "failing-check",
		tier: 1,
		err:  errors.New("check failed"),
	}
	engine.Register(check)

	results, err := engine.Run(context.Background(), fake.NewSimpleClientset())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].MaxSeverity() != SeverityCritical {
		t.Error("failed check should have critical severity")
	}
}

func TestEngineSetConfig(t *testing.T) {
	engine := NewEngine(false)
	cfg := config.DefaultConfig()
	engine.SetConfig(cfg)

	if engine.config != cfg {
		t.Error("config was not set")
	}
}

func TestEngineDisabledCheck(t *testing.T) {
	engine := NewEngine(false)
	cfg := config.DefaultConfig()
	disabled := false
	cfg.Checks["disabled-check"] = config.CheckConfig{Enabled: &disabled}
	engine.SetConfig(cfg)

	check := &mockCheck{name: "disabled-check", tier: 1}
	engine.Register(check)

	results, _ := engine.Run(context.Background(), fake.NewSimpleClientset())
	if check.called {
		t.Error("disabled check should not be called")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for disabled check, got %d", len(results))
	}
}

func TestEngineConfigurableCheck(t *testing.T) {
	engine := NewEngine(false)
	cfg := config.DefaultConfig()
	engine.SetConfig(cfg)

	check := &mockCheck{
		name: "configurable-check",
		tier: 1,
		result: &CheckResult{
			Name:    "configurable-check",
			Tier:    1,
			Results: []Result{},
		},
	}
	engine.Register(check)

	engine.Run(context.Background(), fake.NewSimpleClientset())
	if check.cfgUsed != cfg {
		t.Error("config should be passed to configurable check")
	}
}

func TestEngineMaxSeverity(t *testing.T) {
	engine := NewEngine(false)

	tests := []struct {
		name     string
		results  []CheckResult
		expected Severity
	}{
		{
			name:     "empty",
			results:  []CheckResult{},
			expected: SeverityOK,
		},
		{
			name: "all ok",
			results: []CheckResult{
				{Results: []Result{{Severity: SeverityOK}}},
			},
			expected: SeverityOK,
		},
		{
			name: "with warning",
			results: []CheckResult{
				{Results: []Result{{Severity: SeverityOK}}},
				{Results: []Result{{Severity: SeverityWarning}}},
			},
			expected: SeverityWarning,
		},
		{
			name: "with critical",
			results: []CheckResult{
				{Results: []Result{{Severity: SeverityWarning}}},
				{Results: []Result{{Severity: SeverityCritical}}},
			},
			expected: SeverityCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := engine.MaxSeverity(tt.results)
			if got != tt.expected {
				t.Errorf("MaxSeverity() = %v, want %v", got, tt.expected)
			}
		})
	}
}
