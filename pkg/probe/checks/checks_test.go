package checks

import (
	"context"
	"testing"

	"github.com/punasusi/cluster-probe/pkg/probe"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32Ptr(i int32) *int32 { return &i }
func boolPtr(b bool) *bool    { return &b }

func TestNodeStatus(t *testing.T) {
	check := NewNodeStatus()
	if check.Name() != "node-status" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 1 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.MaxSeverity() != probe.SeverityOK {
		t.Error("healthy node should be OK")
	}
}

func TestNodeStatusNotReady(t *testing.T) {
	check := NewNodeStatus()
	client := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionFalse},
			},
		},
	})

	result, _ := check.Run(context.Background(), client)
	if result.MaxSeverity() != probe.SeverityCritical {
		t.Error("not ready node should be critical")
	}
}

func TestControlPlane(t *testing.T) {
	check := NewControlPlane()
	if check.Name() != "control-plane" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 1 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset()
	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestCriticalPods(t *testing.T) {
	check := NewCriticalPods()
	if check.Name() != "critical-pods" {
		t.Errorf("unexpected name: %s", check.Name())
	}

	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "coredns", Namespace: "kube-system"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestCertificates(t *testing.T) {
	check := NewCertificates()
	if check.Name() != "certificates" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 1 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset()
	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestPodStatus(t *testing.T) {
	check := NewPodStatus()
	if check.Name() != "pod-status" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 2 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod2", Namespace: "default"},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
		},
	)

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.Results) == 0 {
		t.Error("expected at least one result")
	}
}

func TestDeploymentStatus(t *testing.T) {
	check := NewDeploymentStatus()
	if check.Name() != "deployment-status" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 2 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 3, ReadyReplicas: 3, UpdatedReplicas: 3},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.MaxSeverity() != probe.SeverityOK {
		t.Error("healthy deployment should be OK")
	}
}

func TestDeploymentStatusUnhealthy(t *testing.T) {
	check := NewDeploymentStatus()
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: int32Ptr(3)},
		Status:     appsv1.DeploymentStatus{AvailableReplicas: 0, ReadyReplicas: 0},
	})

	result, _ := check.Run(context.Background(), client)
	if result.MaxSeverity() == probe.SeverityOK {
		t.Error("unhealthy deployment should not be OK")
	}
}

func TestPVCStatus(t *testing.T) {
	check := NewPVCStatus()
	if check.Name() != "pvc-status" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 2 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: "pvc1", Namespace: "default"},
		Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.MaxSeverity() != probe.SeverityOK {
		t.Error("bound PVC should be OK")
	}
}

func TestJobFailures(t *testing.T) {
	check := NewJobFailures()
	if check.Name() != "job-failures" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 2 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job1", Namespace: "default"},
		Status:     batchv1.JobStatus{Succeeded: 1},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestResourceRequests(t *testing.T) {
	check := NewResourceRequests()
	if check.Name() != "resource-requests" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 3 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "container1"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestNodeCapacity(t *testing.T) {
	check := NewNodeCapacity()
	if check.Name() != "node-capacity" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 3 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node1"},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestStorageHealth(t *testing.T) {
	check := NewStorageHealth()
	if check.Name() != "storage-health" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 3 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "standard",
			Annotations: map[string]string{"storageclass.kubernetes.io/is-default-class": "true"},
		},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestQuotaUsage(t *testing.T) {
	check := NewQuotaUsage()
	if check.Name() != "quota-usage" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 3 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset()
	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestServiceEndpoints(t *testing.T) {
	check := NewServiceEndpoints()
	if check.Name() != "service-endpoints" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 4 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "default"},
			Spec:       corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "svc1", Namespace: "default"},
			Subsets:    []corev1.EndpointSubset{{Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}}}},
		},
	)

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestIngressStatus(t *testing.T) {
	check := NewIngressStatus()
	if check.Name() != "ingress-status" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 4 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "ingress1", Namespace: "default"},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestNetworkPolicies(t *testing.T) {
	check := NewNetworkPolicies()
	if check.Name() != "network-policies" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 4 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "default"},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestDNSResolution(t *testing.T) {
	check := NewDNSResolution()
	if check.Name() != "dns-resolution" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 4 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset()
	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestRBACAudit(t *testing.T) {
	check := NewRBACAudit()
	if check.Name() != "rbac-audit" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 5 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "test-role"},
		Rules:      []rbacv1.PolicyRule{},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestRBACAuditWildcard(t *testing.T) {
	check := NewRBACAudit()
	client := fake.NewSimpleClientset(&rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-role"},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{"*"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		}},
	})

	result, _ := check.Run(context.Background(), client)
	if result.MaxSeverity() != probe.SeverityWarning {
		t.Error("wildcard role should trigger warning")
	}
}

func TestPodSecurity(t *testing.T) {
	check := NewPodSecurity()
	if check.Name() != "pod-security" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 5 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c1"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestPodSecurityPrivileged(t *testing.T) {
	check := NewPodSecurity()
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{
				Name: "c1",
				SecurityContext: &corev1.SecurityContext{
					Privileged: boolPtr(true),
				},
			}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	})

	result, _ := check.Run(context.Background(), client)
	if result.MaxSeverity() != probe.SeverityWarning {
		t.Error("privileged container should trigger warning")
	}
}

func TestSecretsUsage(t *testing.T) {
	check := NewSecretsUsage()
	if check.Name() != "secrets-usage" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 5 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "c1"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestServiceAccounts(t *testing.T) {
	check := NewServiceAccounts()
	if check.Name() != "service-accounts" {
		t.Errorf("unexpected name: %s", check.Name())
	}
	if check.Tier() != 5 {
		t.Errorf("unexpected tier: %d", check.Tier())
	}

	client := fake.NewSimpleClientset(&corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "default"},
	})

	result, err := check.Run(context.Background(), client)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result == nil {
		t.Fatal("result should not be nil")
	}
}

func TestContainsString(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !containsString(slice, "b") {
		t.Error("should contain 'b'")
	}
	if containsString(slice, "d") {
		t.Error("should not contain 'd'")
	}
}

func TestIsDangerousCapability(t *testing.T) {
	dangerous := []string{"SYS_ADMIN", "NET_ADMIN", "ALL"}
	for _, cap := range dangerous {
		if !isDangerousCapability(cap) {
			t.Errorf("%s should be dangerous", cap)
		}
	}

	if isDangerousCapability("NET_BIND_SERVICE") {
		t.Error("NET_BIND_SERVICE should not be dangerous")
	}
}
