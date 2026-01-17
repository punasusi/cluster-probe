package checks

import (
	"context"
	"fmt"

	"github.com/punasusi/cluster-probe/pkg/probe"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type StorageHealth struct{}

func NewStorageHealth() *StorageHealth {
	return &StorageHealth{}
}

func (c *StorageHealth) Name() string {
	return "storage-health"
}

func (c *StorageHealth) Tier() int {
	return 3
}

func (c *StorageHealth) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	storageClasses, err := client.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list storage classes: %w", err)
	}

	var defaultSC *storagev1.StorageClass
	scCount := len(storageClasses.Items)

	for i := range storageClasses.Items {
		sc := &storageClasses.Items[i]
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			if defaultSC != nil {
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	"Multiple default storage classes defined",
					Details:	[]string{fmt.Sprintf("Found: %s and %s", defaultSC.Name, sc.Name)},
					Remediation:	"Only one storage class should be marked as default",
				})
			}
			defaultSC = sc
		}
	}

	if scCount > 0 && defaultSC == nil {
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityWarning,
			Message:	"No default storage class defined",
			Details:	[]string{fmt.Sprintf("Available storage classes: %d", scCount)},
			Remediation:	"Mark a storage class as default: kubectl patch storageclass <name> -p '{\"metadata\":{\"annotations\":{\"storageclass.kubernetes.io/is-default-class\":\"true\"}}}'",
		})
	}

	volumeAttachments, err := client.StorageV1().VolumeAttachments().List(ctx, metav1.ListOptions{})
	if err == nil {
		attachedCount := 0
		attachingCount := 0
		failedCount := 0

		for _, va := range volumeAttachments.Items {
			if va.Status.Attached {
				attachedCount++
			} else if va.Status.AttachError != nil {
				failedCount++
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityCritical,
					Message:	fmt.Sprintf("Volume attachment %s failed", va.Name),
					Details: []string{
						fmt.Sprintf("Node: %s", va.Spec.NodeName),
						fmt.Sprintf("Error: %s", va.Status.AttachError.Message),
					},
					Remediation:	"Check CSI driver logs and node storage connectivity",
				})
			} else {
				attachingCount++
			}
		}

		if attachingCount > 0 {
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("%d volume attachments are pending", attachingCount),
				Remediation:	"Check CSI driver status and node availability",
			})
		}
	}

	csiDrivers, err := client.StorageV1().CSIDrivers().List(ctx, metav1.ListOptions{})
	if err == nil && len(csiDrivers.Items) > 0 {
		driverNames := make([]string, 0, len(csiDrivers.Items))
		for _, driver := range csiDrivers.Items {
			driverNames = append(driverNames, driver.Name)
		}
		result.Results = append(result.Results, probe.Result{
			CheckName:	c.Name(),
			Severity:	probe.SeverityOK,
			Message:	fmt.Sprintf("%d CSI drivers installed", len(csiDrivers.Items)),
			Details:	driverNames,
		})
	}

	severity := probe.SeverityOK
	details := []string{fmt.Sprintf("Storage classes: %d", scCount)}
	if defaultSC != nil {
		details = append(details, fmt.Sprintf("Default: %s", defaultSC.Name))
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	"Storage configuration summary",
		Details:	details,
	})

	return result, nil
}
