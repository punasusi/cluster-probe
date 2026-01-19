package checks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/punasusi/cluster-probe/pkg/probe"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

type stalledStats struct {
	pendingPods        int
	backoffPods        int
	pendingPVCs        int
	pendingPVs         int
	stalledDeploys     int
	stalledStateful    int
	stalledDaemonSets  int
	stalledReplicaSets int
	backoffJobs        int
	stalledCRs         int
}

type StalledResources struct{}

func NewStalledResources() *StalledResources {
	return &StalledResources{}
}

func (c *StalledResources) Name() string {
	return "stalled-resources"
}

func (c *StalledResources) Tier() int {
	return 2
}

func (c *StalledResources) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:    c.Name(),
		Tier:    c.Tier(),
		Results: []probe.Result{},
	}

	stats := &stalledStats{}

	c.checkPods(ctx, client, result, stats)
	c.checkPVCs(ctx, client, result, stats)
	c.checkPVs(ctx, client, result, stats)
	c.checkDeployments(ctx, client, result, stats)
	c.checkStatefulSets(ctx, client, result, stats)
	c.checkDaemonSets(ctx, client, result, stats)
	c.checkReplicaSets(ctx, client, result, stats)
	c.checkJobs(ctx, client, result, stats)

	c.appendSummary(result, stats)

	return result, nil
}

func (c *StalledResources) RunDynamic(ctx context.Context, client kubernetes.Interface, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:    c.Name(),
		Tier:    c.Tier(),
		Results: []probe.Result{},
	}

	stats := &stalledStats{}

	c.checkPods(ctx, client, result, stats)
	c.checkPVCs(ctx, client, result, stats)
	c.checkPVs(ctx, client, result, stats)
	c.checkDeployments(ctx, client, result, stats)
	c.checkStatefulSets(ctx, client, result, stats)
	c.checkDaemonSets(ctx, client, result, stats)
	c.checkReplicaSets(ctx, client, result, stats)
	c.checkJobs(ctx, client, result, stats)

	c.checkCustomResources(ctx, dynamicClient, discoveryClient, result, stats)

	c.appendSummary(result, stats)

	return result, nil
}

func (c *StalledResources) appendSummary(result *probe.CheckResult, stats *stalledStats) {
	total := stats.pendingPods + stats.backoffPods + stats.pendingPVCs + stats.pendingPVs +
		stats.stalledDeploys + stats.stalledStateful + stats.stalledDaemonSets +
		stats.stalledReplicaSets + stats.backoffJobs + stats.stalledCRs

	severity := probe.SeverityOK
	if total > 0 {
		severity = probe.SeverityWarning
	}
	if stats.backoffPods > 3 || stats.backoffJobs > 0 {
		severity = probe.SeverityCritical
	}

	details := []string{}
	if stats.pendingPods > 0 {
		details = append(details, fmt.Sprintf("Pending pods: %d", stats.pendingPods))
	}
	if stats.backoffPods > 0 {
		details = append(details, fmt.Sprintf("Backoff pods: %d", stats.backoffPods))
	}
	if stats.pendingPVCs > 0 {
		details = append(details, fmt.Sprintf("Pending PVCs: %d", stats.pendingPVCs))
	}
	if stats.pendingPVs > 0 {
		details = append(details, fmt.Sprintf("Pending PVs: %d", stats.pendingPVs))
	}
	if stats.stalledDeploys > 0 {
		details = append(details, fmt.Sprintf("Stalled Deployments: %d", stats.stalledDeploys))
	}
	if stats.stalledStateful > 0 {
		details = append(details, fmt.Sprintf("Stalled StatefulSets: %d", stats.stalledStateful))
	}
	if stats.stalledDaemonSets > 0 {
		details = append(details, fmt.Sprintf("Stalled DaemonSets: %d", stats.stalledDaemonSets))
	}
	if stats.stalledReplicaSets > 0 {
		details = append(details, fmt.Sprintf("Stalled ReplicaSets: %d", stats.stalledReplicaSets))
	}
	if stats.backoffJobs > 0 {
		details = append(details, fmt.Sprintf("Backoff Jobs: %d", stats.backoffJobs))
	}
	if stats.stalledCRs > 0 {
		details = append(details, fmt.Sprintf("Stalled custom resources: %d", stats.stalledCRs))
	}

	if len(details) == 0 {
		details = append(details, "No stalled resources found")
	}

	result.Results = append(result.Results, probe.Result{
		CheckName: c.Name(),
		Severity:  severity,
		Message:   fmt.Sprintf("Stalled resources: %d total", total),
		Details:   details,
	})
}

func (c *StalledResources) checkCustomResources(ctx context.Context, dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface, result *probe.CheckResult, stats *stalledStats) {
	_, apiResourceLists, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return
	}

	coreGroups := map[string]bool{
		"":                          true,
		"apps":                      true,
		"batch":                     true,
		"autoscaling":               true,
		"policy":                    true,
		"networking.k8s.io":         true,
		"storage.k8s.io":            true,
		"rbac.authorization.k8s.io": true,
		"admissionregistration.k8s.io": true,
		"apiextensions.k8s.io":      true,
		"certificates.k8s.io":       true,
		"coordination.k8s.io":       true,
		"discovery.k8s.io":          true,
		"events.k8s.io":             true,
		"flowcontrol.apiserver.k8s.io": true,
		"node.k8s.io":               true,
		"scheduling.k8s.io":         true,
	}

	for _, apiResourceList := range apiResourceLists {
		gv, err := schema.ParseGroupVersion(apiResourceList.GroupVersion)
		if err != nil {
			continue
		}

		if coreGroups[gv.Group] {
			continue
		}

		for _, apiResource := range apiResourceList.APIResources {
			if !c.canListResource(apiResource) {
				continue
			}

			if strings.Contains(apiResource.Name, "/") {
				continue
			}

			gvr := schema.GroupVersionResource{
				Group:    gv.Group,
				Version:  gv.Version,
				Resource: apiResource.Name,
			}

			c.checkResourcesForGVR(ctx, dynamicClient, gvr, apiResource.Namespaced, apiResource.Kind, result, stats)
		}
	}
}

func (c *StalledResources) canListResource(apiResource metav1.APIResource) bool {
	for _, verb := range apiResource.Verbs {
		if verb == "list" {
			return true
		}
	}
	return false
}

func (c *StalledResources) checkResourcesForGVR(ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespaced bool, kind string, result *probe.CheckResult, stats *stalledStats) {
	var list *unstructured.UnstructuredList
	var err error

	if namespaced {
		list, err = dynamicClient.Resource(gvr).Namespace("").List(ctx, metav1.ListOptions{})
	} else {
		list, err = dynamicClient.Resource(gvr).List(ctx, metav1.ListOptions{})
	}

	if err != nil {
		return
	}

	for _, item := range list.Items {
		c.checkResourceStatus(&item, kind, gvr, result, stats)
	}
}

func (c *StalledResources) checkResourceStatus(item *unstructured.Unstructured, kind string, gvr schema.GroupVersionResource, result *probe.CheckResult, stats *stalledStats) {
	status, found, err := unstructured.NestedMap(item.Object, "status")
	if err != nil || !found {
		return
	}

	name := item.GetName()
	namespace := item.GetNamespace()
	resourceID := c.formatResourceID(namespace, name, kind, gvr.Group)

	if phase, found, _ := unstructured.NestedString(status, "phase"); found {
		if c.isStalledPhase(phase) {
			stats.stalledCRs++
			result.Results = append(result.Results, probe.Result{
				CheckName:   c.Name(),
				Severity:    probe.SeverityWarning,
				Message:     fmt.Sprintf("%s in %s phase", resourceID, phase),
				Details:     c.extractStatusDetails(status),
				Remediation: fmt.Sprintf("kubectl describe %s %s", c.formatResourceType(kind, gvr, namespace), c.formatResourceRef(namespace, name)),
			})
			return
		}
	}

	if state, found, _ := unstructured.NestedString(status, "state"); found {
		if c.isStalledPhase(state) {
			stats.stalledCRs++
			result.Results = append(result.Results, probe.Result{
				CheckName:   c.Name(),
				Severity:    probe.SeverityWarning,
				Message:     fmt.Sprintf("%s in %s state", resourceID, state),
				Details:     c.extractStatusDetails(status),
				Remediation: fmt.Sprintf("kubectl describe %s %s", c.formatResourceType(kind, gvr, namespace), c.formatResourceRef(namespace, name)),
			})
			return
		}
	}

	conditions, found, _ := unstructured.NestedSlice(status, "conditions")
	if found {
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}

			condType, _ := condMap["type"].(string)
			condStatus, _ := condMap["status"].(string)
			reason, _ := condMap["reason"].(string)

			if c.isStalledCondition(condType, condStatus, reason) {
				stats.stalledCRs++
				details := c.extractConditionDetails(condMap)
				result.Results = append(result.Results, probe.Result{
					CheckName:   c.Name(),
					Severity:    probe.SeverityWarning,
					Message:     fmt.Sprintf("%s has %s=%s", resourceID, condType, condStatus),
					Details:     details,
					Remediation: fmt.Sprintf("kubectl describe %s %s", c.formatResourceType(kind, gvr, namespace), c.formatResourceRef(namespace, name)),
				})
				return
			}
		}
	}
}

func (c *StalledResources) isStalledPhase(phase string) bool {
	stalledPhases := map[string]bool{
		"Pending":      true,
		"pending":      true,
		"Waiting":      true,
		"waiting":      true,
		"Failed":       true,
		"failed":       true,
		"Error":        true,
		"error":        true,
		"Errored":      true,
		"errored":      true,
		"Unknown":      true,
		"unknown":      true,
		"Terminating":  true,
		"terminating":  true,
		"Provisioning": true,
		"provisioning": true,
		"Degraded":     true,
		"degraded":     true,
		"Unhealthy":    true,
		"unhealthy":    true,
		"NotReady":     true,
		"notready":     true,
		"Stalled":      true,
		"stalled":      true,
		"Blocked":      true,
		"blocked":      true,
	}
	return stalledPhases[phase]
}

func (c *StalledResources) isStalledCondition(condType, status, reason string) bool {
	lowerType := strings.ToLower(condType)
	lowerReason := strings.ToLower(reason)

	if (lowerType == "ready" || lowerType == "available" || lowerType == "healthy") && status == "False" {
		return true
	}

	if (lowerType == "failed" || lowerType == "error" || lowerType == "degraded") && status == "True" {
		return true
	}

	stalledReasons := []string{"backoff", "failed", "error", "timeout", "pending", "waiting", "degraded"}
	for _, r := range stalledReasons {
		if strings.Contains(lowerReason, r) {
			return true
		}
	}

	return false
}

func (c *StalledResources) extractStatusDetails(status map[string]interface{}) []string {
	details := []string{}

	if message, found, _ := unstructured.NestedString(status, "message"); found && message != "" {
		details = append(details, fmt.Sprintf("Message: %s", message))
	}

	if reason, found, _ := unstructured.NestedString(status, "reason"); found && reason != "" {
		details = append(details, fmt.Sprintf("Reason: %s", reason))
	}

	return details
}

func (c *StalledResources) extractConditionDetails(condMap map[string]interface{}) []string {
	details := []string{}

	if reason, ok := condMap["reason"].(string); ok && reason != "" {
		details = append(details, fmt.Sprintf("Reason: %s", reason))
	}

	if message, ok := condMap["message"].(string); ok && message != "" {
		details = append(details, fmt.Sprintf("Message: %s", message))
	}

	return details
}

func (c *StalledResources) formatResourceID(namespace, name, kind, group string) string {
	resourceType := kind
	if group != "" {
		resourceType = fmt.Sprintf("%s.%s", kind, group)
	}

	if namespace != "" {
		return fmt.Sprintf("%s %s/%s", resourceType, namespace, name)
	}
	return fmt.Sprintf("%s %s", resourceType, name)
}

func (c *StalledResources) formatResourceType(kind string, gvr schema.GroupVersionResource, namespace string) string {
	if gvr.Group != "" {
		return fmt.Sprintf("%s.%s", strings.ToLower(kind), gvr.Group)
	}
	return strings.ToLower(kind)
}

func (c *StalledResources) formatResourceRef(namespace, name string) string {
	if namespace != "" {
		return fmt.Sprintf("-n %s %s", namespace, name)
	}
	return name
}

func (c *StalledResources) checkPods(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	pods, err := client.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodPending {
			age := time.Since(pod.CreationTimestamp.Time)
			if age > 5*time.Minute {
				stats.pendingPods++
				result.Results = append(result.Results, probe.Result{
					CheckName:   c.Name(),
					Severity:    probe.SeverityWarning,
					Message:     fmt.Sprintf("Pod %s/%s pending for %s", pod.Namespace, pod.Name, stalledFormatDuration(age)),
					Details:     c.getPendingPodDetails(&pod),
					Remediation: "Check node resources, scheduling constraints, and pod events",
				})
			}
		}

		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				if c.isBackoffReason(reason) {
					stats.backoffPods++
					result.Results = append(result.Results, probe.Result{
						CheckName: c.Name(),
						Severity:  probe.SeverityWarning,
						Message:   fmt.Sprintf("Pod %s/%s container %s in %s", pod.Namespace, pod.Name, cs.Name, reason),
						Details: []string{
							fmt.Sprintf("Restarts: %d", cs.RestartCount),
							fmt.Sprintf("Message: %s", cs.State.Waiting.Message),
						},
						Remediation: c.getBackoffRemediation(reason, pod.Namespace, pod.Name, cs.Name),
					})
				}
			}
		}

		for _, cs := range pod.Status.InitContainerStatuses {
			if cs.State.Waiting != nil {
				reason := cs.State.Waiting.Reason
				if c.isBackoffReason(reason) {
					stats.backoffPods++
					result.Results = append(result.Results, probe.Result{
						CheckName: c.Name(),
						Severity:  probe.SeverityWarning,
						Message:   fmt.Sprintf("Pod %s/%s init container %s in %s", pod.Namespace, pod.Name, cs.Name, reason),
						Details: []string{
							fmt.Sprintf("Restarts: %d", cs.RestartCount),
							fmt.Sprintf("Message: %s", cs.State.Waiting.Message),
						},
						Remediation: c.getBackoffRemediation(reason, pod.Namespace, pod.Name, cs.Name),
					})
				}
			}
		}
	}
}

func (c *StalledResources) checkPVCs(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	pvcs, err := client.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase == corev1.ClaimPending {
			age := time.Since(pvc.CreationTimestamp.Time)
			if age > 2*time.Minute {
				stats.pendingPVCs++
				details := []string{
					fmt.Sprintf("Age: %s", stalledFormatDuration(age)),
				}
				if pvc.Spec.StorageClassName != nil {
					details = append(details, fmt.Sprintf("StorageClass: %s", *pvc.Spec.StorageClassName))
				}
				result.Results = append(result.Results, probe.Result{
					CheckName:   c.Name(),
					Severity:    probe.SeverityWarning,
					Message:     fmt.Sprintf("PVC %s/%s pending for %s", pvc.Namespace, pvc.Name, stalledFormatDuration(age)),
					Details:     details,
					Remediation: "Check storage provisioner and available capacity",
				})
			}
		}
	}
}

func (c *StalledResources) checkPVs(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	pvs, err := client.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, pv := range pvs.Items {
		if pv.Status.Phase == corev1.VolumePending {
			age := time.Since(pv.CreationTimestamp.Time)
			if age > 2*time.Minute {
				stats.pendingPVs++
				result.Results = append(result.Results, probe.Result{
					CheckName:   c.Name(),
					Severity:    probe.SeverityWarning,
					Message:     fmt.Sprintf("PV %s pending for %s", pv.Name, stalledFormatDuration(age)),
					Details:     []string{fmt.Sprintf("Age: %s", stalledFormatDuration(age))},
					Remediation: "Check PV configuration and storage backend",
				})
			}
		}
	}
}

func (c *StalledResources) checkDeployments(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	deploys, err := client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, deploy := range deploys.Items {
		if deploy.Spec.Replicas == nil {
			continue
		}
		desired := *deploy.Spec.Replicas
		if desired == 0 {
			continue
		}

		unavailable := deploy.Status.UnavailableReplicas
		if unavailable > 0 {
			for _, cond := range deploy.Status.Conditions {
				if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionFalse {
					stats.stalledDeploys++
					result.Results = append(result.Results, probe.Result{
						CheckName: c.Name(),
						Severity:  probe.SeverityWarning,
						Message:   fmt.Sprintf("Deployment %s/%s stalled with %d unavailable replicas", deploy.Namespace, deploy.Name, unavailable),
						Details: []string{
							fmt.Sprintf("Desired: %d, Available: %d, Unavailable: %d", desired, deploy.Status.AvailableReplicas, unavailable),
							fmt.Sprintf("Reason: %s", cond.Reason),
						},
						Remediation: "Check deployment events and pod status",
					})
					break
				}
			}
		}
	}
}

func (c *StalledResources) checkStatefulSets(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	statefulsets, err := client.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, sts := range statefulsets.Items {
		if sts.Spec.Replicas == nil {
			continue
		}
		desired := *sts.Spec.Replicas
		if desired == 0 {
			continue
		}

		ready := sts.Status.ReadyReplicas
		if ready < desired {
			age := time.Since(sts.CreationTimestamp.Time)
			if age > 5*time.Minute && sts.Status.UpdatedReplicas == sts.Status.Replicas {
				stats.stalledStateful++
				result.Results = append(result.Results, probe.Result{
					CheckName: c.Name(),
					Severity:  probe.SeverityWarning,
					Message:   fmt.Sprintf("StatefulSet %s/%s has %d/%d ready replicas", sts.Namespace, sts.Name, ready, desired),
					Details: []string{
						fmt.Sprintf("Current: %d, Ready: %d, Desired: %d", sts.Status.CurrentReplicas, ready, desired),
					},
					Remediation: "Check StatefulSet events and pod status",
				})
			}
		}
	}
}

func (c *StalledResources) checkDaemonSets(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	daemonsets, err := client.AppsV1().DaemonSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, ds := range daemonsets.Items {
		unavailable := ds.Status.NumberUnavailable
		if unavailable > 0 {
			age := time.Since(ds.CreationTimestamp.Time)
			if age > 5*time.Minute {
				stats.stalledDaemonSets++
				result.Results = append(result.Results, probe.Result{
					CheckName: c.Name(),
					Severity:  probe.SeverityWarning,
					Message:   fmt.Sprintf("DaemonSet %s/%s has %d unavailable pods", ds.Namespace, ds.Name, unavailable),
					Details: []string{
						fmt.Sprintf("Desired: %d, Ready: %d, Unavailable: %d", ds.Status.DesiredNumberScheduled, ds.Status.NumberReady, unavailable),
					},
					Remediation: "Check DaemonSet events and node status",
				})
			}
		}
	}
}

func (c *StalledResources) checkReplicaSets(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	replicasets, err := client.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, rs := range replicasets.Items {
		if rs.Spec.Replicas == nil {
			continue
		}
		desired := *rs.Spec.Replicas
		if desired == 0 {
			continue
		}

		if len(rs.OwnerReferences) > 0 {
			continue
		}

		ready := rs.Status.ReadyReplicas
		if ready < desired {
			age := time.Since(rs.CreationTimestamp.Time)
			if age > 5*time.Minute {
				stats.stalledReplicaSets++
				result.Results = append(result.Results, probe.Result{
					CheckName: c.Name(),
					Severity:  probe.SeverityWarning,
					Message:   fmt.Sprintf("ReplicaSet %s/%s has %d/%d ready replicas", rs.Namespace, rs.Name, ready, desired),
					Details: []string{
						fmt.Sprintf("Replicas: %d, Ready: %d, Available: %d", rs.Status.Replicas, ready, rs.Status.AvailableReplicas),
					},
					Remediation: "Check ReplicaSet events and pod status",
				})
			}
		}
	}
}

func (c *StalledResources) checkJobs(ctx context.Context, client kubernetes.Interface, result *probe.CheckResult, stats *stalledStats) {
	jobs, err := client.BatchV1().Jobs("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return
	}

	for _, job := range jobs.Items {
		for _, cond := range job.Status.Conditions {
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				if cond.Reason == "BackoffLimitExceeded" {
					stats.backoffJobs++
					result.Results = append(result.Results, probe.Result{
						CheckName: c.Name(),
						Severity:  probe.SeverityCritical,
						Message:   fmt.Sprintf("Job %s/%s exceeded backoff limit", job.Namespace, job.Name),
						Details: []string{
							fmt.Sprintf("Failed: %d", job.Status.Failed),
							fmt.Sprintf("Message: %s", cond.Message),
						},
						Remediation: fmt.Sprintf("Check job logs and events: kubectl describe job -n %s %s", job.Namespace, job.Name),
					})
				}
			}
		}
	}
}

func (c *StalledResources) isBackoffReason(reason string) bool {
	backoffReasons := map[string]bool{
		"CrashLoopBackOff":           true,
		"ImagePullBackOff":           true,
		"ErrImagePull":               true,
		"CreateContainerError":       true,
		"InvalidImageName":           true,
		"CreateContainerConfigError": true,
	}
	return backoffReasons[reason]
}

func (c *StalledResources) getBackoffRemediation(reason, namespace, podName, containerName string) string {
	switch reason {
	case "CrashLoopBackOff":
		return fmt.Sprintf("Check logs: kubectl logs -n %s %s -c %s --previous", namespace, podName, containerName)
	case "ImagePullBackOff", "ErrImagePull", "InvalidImageName":
		return "Verify image name, registry credentials, and network access"
	case "CreateContainerError", "CreateContainerConfigError":
		return fmt.Sprintf("Check pod events: kubectl describe pod -n %s %s", namespace, podName)
	default:
		return fmt.Sprintf("Check pod status: kubectl describe pod -n %s %s", namespace, podName)
	}
}

func (c *StalledResources) getPendingPodDetails(pod *corev1.Pod) []string {
	details := []string{}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			details = append(details, fmt.Sprintf("Unschedulable: %s", cond.Message))
		}
	}
	if len(details) == 0 {
		details = append(details, "Waiting for scheduling or resources")
	}
	return details
}

func stalledFormatDuration(d time.Duration) string {
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
