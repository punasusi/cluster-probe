package checks

import (
	"context"
	"fmt"
	"time"

	"github.com/punasusi/cluster-probe/pkg/probe"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type PVCStatus struct{}

func NewPVCStatus() *PVCStatus {
	return &PVCStatus{}
}

func (c *PVCStatus) Name() string {
	return "pvc-status"
}

func (c *PVCStatus) Tier() int {
	return 2
}

func (c *PVCStatus) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	pvcs, err := client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list PVCs: %w", err)
	}

	bound := 0
	pending := 0
	lost := 0

	for _, pvc := range pvcs.Items {
		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			bound++
		case corev1.ClaimPending:
			pending++
			age := time.Since(pvc.CreationTimestamp.Time)

			severity := probe.SeverityWarning
			if age > 10*time.Minute {
				severity = probe.SeverityCritical
			}

			details := []string{
				fmt.Sprintf("Age: %s", formatDuration(age)),
			}
			if pvc.Spec.StorageClassName != nil {
				details = append(details, fmt.Sprintf("StorageClass: %s", *pvc.Spec.StorageClassName))
			}
			if pvc.Spec.VolumeName != "" {
				details = append(details, fmt.Sprintf("Volume: %s", pvc.Spec.VolumeName))
			}

			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	severity,
				Message:	fmt.Sprintf("PVC %s/%s is pending", pvc.Namespace, pvc.Name),
				Details:	details,
				Remediation:	"Check storage provisioner logs and available PVs",
			})
		case corev1.ClaimLost:
			lost++
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityCritical,
				Message:	fmt.Sprintf("PVC %s/%s has lost its bound volume", pvc.Namespace, pvc.Name),
				Details: []string{
					fmt.Sprintf("Volume was: %s", pvc.Spec.VolumeName),
				},
				Remediation:	"The underlying PV was deleted. Data may be lost. Review and recreate the PVC if needed",
			})
		}
	}

	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err == nil {
		failedPVs := 0
		for _, pv := range pvs.Items {
			if pv.Status.Phase == corev1.VolumeFailed {
				failedPVs++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("PV %s is in Failed state", pv.Name),
					Details: []string{
						fmt.Sprintf("Reason: %s", pv.Status.Reason),
						fmt.Sprintf("Message: %s", pv.Status.Message),
					},
					Remediation:	"Check storage backend and PV configuration",
				})
			}
		}
	}

	severity := probe.SeverityOK
	if pending > 0 || lost > 0 {
		severity = probe.SeverityWarning
	}
	if lost > 0 {
		severity = probe.SeverityCritical
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	fmt.Sprintf("PVC status: %d bound, %d pending, %d lost", bound, pending, lost),
		Details: []string{
			fmt.Sprintf("Total PVCs: %d", len(pvcs.Items)),
		},
	})

	return result, nil
}
