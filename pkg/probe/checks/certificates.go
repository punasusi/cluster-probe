package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/punasusi/cluster-probe/pkg/probe"
	certv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Certificates struct{}

func NewCertificates() *Certificates {
	return &Certificates{}
}

func (c *Certificates) Name() string {
	return "certificates"
}

func (c *Certificates) Tier() int {
	return 1
}

func (c *Certificates) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	csrs, err := client.CertificatesV1().CertificateSigningRequests().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list CSRs: %w", err)
	}

	pendingCSRs := 0
	deniedCSRs := 0
	failedCSRs := 0

	for _, csr := range csrs.Items {
		isPending := true
		for _, cond := range csr.Status.Conditions {
			switch cond.Type {
			case certv1.CertificateApproved:
				isPending = false
			case certv1.CertificateDenied:
				isPending = false
				deniedCSRs++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("CSR %s was denied", csr.Name),
					Details: []string{
						fmt.Sprintf("Reason: %s", cond.Reason),
						fmt.Sprintf("Message: %s", cond.Message),
						fmt.Sprintf("Requestor: %s", csr.Spec.Username),
					},
					Remediation:	"Review the CSR and resubmit if appropriate",
				})
			case certv1.CertificateFailed:
				isPending = false
				failedCSRs++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("CSR %s failed", csr.Name),
					Details: []string{
						fmt.Sprintf("Reason: %s", cond.Reason),
						fmt.Sprintf("Message: %s", cond.Message),
					},
					Remediation:	"Check the CSR request and signer configuration",
				})
			}
		}

		if isPending && len(csr.Status.Conditions) == 0 {

			age := time.Since(csr.CreationTimestamp.Time)
			if age > 1*time.Hour {
				pendingCSRs++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("CSR %s is pending for %s", csr.Name, formatDuration(age)),
					Details: []string{
						fmt.Sprintf("Requestor: %s", csr.Spec.Username),
						fmt.Sprintf("Signer: %s", csr.Spec.SignerName),
					},
					Remediation:	"Review and approve/deny the CSR: kubectl certificate approve/deny " + csr.Name,
				})
			}
		}

		if len(csr.Status.Certificate) > 0 {

		}
	}

	serverVersion, err := client.Discovery().ServerVersion()
	if err != nil {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityCritical,
			Message:	"Cannot verify API server certificate",
			Details:	[]string{err.Error()},
			Remediation:	"Check API server certificate configuration",
		})
	}

	if pendingCSRs == 0 && deniedCSRs == 0 && failedCSRs == 0 && err == nil {
		details := []string{}
		if serverVersion != nil {
			details = append(details, fmt.Sprintf("API server: %s", serverVersion.GitVersion))
		}
		details = append(details, fmt.Sprintf("Total CSRs: %d", len(csrs.Items)))

		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	"Certificate status is healthy",
			Details:	details,
		})
	}

	return result, nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
