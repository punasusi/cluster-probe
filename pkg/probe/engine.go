package probe

import (
	"context"
	"sync"

	"github.com/punasusi/cluster-probe/pkg/probe/config"
	"k8s.io/client-go/kubernetes"
)

type Check interface {
	Name() string
	Tier() int
	Run(ctx context.Context, client kubernetes.Interface) (*CheckResult, error)
}

type ConfigurableCheck interface {
	Check
	Configure(cfg *config.Config)
}

type Engine struct {
	checks	[]Check
	verbose	bool
	config	*config.Config
}

func NewEngine(verbose bool) *Engine {
	return &Engine{
		checks:		make([]Check, 0),
		verbose:	verbose,
		config:		config.DefaultConfig(),
	}
}

func (e *Engine) SetConfig(cfg *config.Config) {
	e.config = cfg
}

func (e *Engine) Register(check Check) {
	e.checks = append(e.checks, check)
}

func (e *Engine) Run(ctx context.Context, client kubernetes.Interface) ([]CheckResult, error) {
	if len(e.checks) == 0 {
		return []CheckResult{}, nil
	}

	results := make([]CheckResult, 0, len(e.checks))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, check := range e.checks {

		if e.config != nil && !e.config.IsCheckEnabled(check.Name()) {
			continue
		}

		if configurable, ok := check.(ConfigurableCheck); ok && e.config != nil {
			configurable.Configure(e.config)
		}

		wg.Add(1)
		go func(c Check) {
			defer wg.Done()
			result, err := c.Run(ctx, client)
			if err != nil {
				mu.Lock()
				results = append(results, CheckResult{
					Name:	c.Name(),
					Tier:	c.Tier(),
					Results: []Result{{
						CheckName:	c.Name(),
						Severity:	SeverityCritical,
						Message:	"Check failed to execute",
						Details:	[]string{err.Error()},
					}},
				})
				mu.Unlock()
				return
			}

			if e.config != nil {
				filteredResults := make([]Result, 0, len(result.Results))
				for _, r := range result.Results {

					ignore := false
					for _, ns := range e.config.Ignore.Namespaces {
						for _, detail := range r.Details {
							if containsNamespace(detail, ns) {
								ignore = true
								break
							}
						}

						if containsNamespace(r.Message, ns) {
							ignore = true
						}
						if ignore {
							break
						}
					}
					if !ignore {
						filteredResults = append(filteredResults, r)
					}
				}
				result.Results = filteredResults
			}

			mu.Lock()
			results = append(results, *result)
			mu.Unlock()
		}(check)
	}

	wg.Wait()
	return results, nil
}

func containsNamespace(s, ns string) bool {

	return len(s) > len(ns)+1 && (s[:len(ns)+1] == ns+"/" ||
		(len(s) > len(ns) && s[len(s)-len(ns)-1:] == "/"+ns))
}

func (e *Engine) MaxSeverity(results []CheckResult) Severity {
	max := SeverityOK
	for _, r := range results {
		if s := r.MaxSeverity(); s > max {
			max = s
		}
	}
	return max
}
