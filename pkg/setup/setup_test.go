package setup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewSetup(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "/path/to/kubeconfig", true)

	if s == nil {
		t.Fatal("NewSetup returned nil")
	}
	if s.client != client {
		t.Error("client not set correctly")
	}
	if s.kubeconfigPath != "/path/to/kubeconfig" {
		t.Error("kubeconfigPath not set correctly")
	}
	if !s.verbose {
		t.Error("verbose not set correctly")
	}
}

func TestCreateServiceAccount(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createServiceAccount(ctx); err != nil {
		t.Fatalf("createServiceAccount failed: %v", err)
	}

	sa, err := client.CoreV1().ServiceAccounts(ServiceAccountNamespace).Get(ctx, ServiceAccountName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created service account: %v", err)
	}

	if sa.Name != ServiceAccountName {
		t.Errorf("unexpected service account name: %s", sa.Name)
	}
	if sa.Labels["app.kubernetes.io/name"] != "cluster-probe" {
		t.Error("missing expected label")
	}
}

func TestCreateServiceAccountAlreadyExists(t *testing.T) {
	existingSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ServiceAccountName,
			Namespace: ServiceAccountNamespace,
		},
	}
	client := fake.NewSimpleClientset(existingSA)
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createServiceAccount(ctx); err != nil {
		t.Errorf("createServiceAccount should not error when SA exists: %v", err)
	}
}

func TestCreateClusterRole(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "", false)
	ctx := context.Background()

	crdGroups := []string{"custom.example.com", "apps.example.com"}
	if err := s.createClusterRole(ctx, crdGroups); err != nil {
		t.Fatalf("createClusterRole failed: %v", err)
	}

	role, err := client.RbacV1().ClusterRoles().Get(ctx, ClusterRoleName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created cluster role: %v", err)
	}

	if role.Name != ClusterRoleName {
		t.Errorf("unexpected cluster role name: %s", role.Name)
	}

	foundCRDGroups := 0
	for _, rule := range role.Rules {
		for _, group := range rule.APIGroups {
			if group == "custom.example.com" || group == "apps.example.com" {
				foundCRDGroups++
			}
		}
	}
	if foundCRDGroups != 2 {
		t.Errorf("expected 2 CRD groups in rules, found %d", foundCRDGroups)
	}
}

func TestCreateClusterRoleAlreadyExists(t *testing.T) {
	existingRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClusterRoleName,
		},
		Rules: []rbacv1.PolicyRule{},
	}
	client := fake.NewSimpleClientset(existingRole)
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createClusterRole(ctx, []string{}); err != nil {
		t.Errorf("createClusterRole should update when exists: %v", err)
	}

	role, _ := client.RbacV1().ClusterRoles().Get(ctx, ClusterRoleName, metav1.GetOptions{})
	if len(role.Rules) == 0 {
		t.Error("rules should have been updated")
	}
}

func TestCreateClusterRoleBinding(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createClusterRoleBinding(ctx); err != nil {
		t.Fatalf("createClusterRoleBinding failed: %v", err)
	}

	binding, err := client.RbacV1().ClusterRoleBindings().Get(ctx, ClusterRoleBindingName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created binding: %v", err)
	}

	if binding.RoleRef.Name != ClusterRoleName {
		t.Errorf("unexpected role ref: %s", binding.RoleRef.Name)
	}
	if len(binding.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(binding.Subjects))
	}
	if binding.Subjects[0].Name != ServiceAccountName {
		t.Errorf("unexpected subject name: %s", binding.Subjects[0].Name)
	}
}

func TestCreateClusterRoleBindingAlreadyExists(t *testing.T) {
	existingBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: ClusterRoleBindingName,
		},
	}
	client := fake.NewSimpleClientset(existingBinding)
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createClusterRoleBinding(ctx); err != nil {
		t.Errorf("createClusterRoleBinding should not error when exists: %v", err)
	}
}

func TestCreateTokenSecret(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createTokenSecret(ctx); err != nil {
		t.Fatalf("createTokenSecret failed: %v", err)
	}

	secret, err := client.CoreV1().Secrets(ServiceAccountNamespace).Get(ctx, TokenSecretName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("failed to get created secret: %v", err)
	}

	if secret.Type != corev1.SecretTypeServiceAccountToken {
		t.Errorf("unexpected secret type: %s", secret.Type)
	}
	if secret.Annotations["kubernetes.io/service-account.name"] != ServiceAccountName {
		t.Error("missing service account annotation")
	}
}

func TestCreateTokenSecretAlreadyExists(t *testing.T) {
	existingSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TokenSecretName,
			Namespace: ServiceAccountNamespace,
		},
	}
	client := fake.NewSimpleClientset(existingSecret)
	s := NewSetup(client, "", false)
	ctx := context.Background()

	if err := s.createTokenSecret(ctx); err != nil {
		t.Errorf("createTokenSecret should not error when exists: %v", err)
	}
}

func TestGetToken(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TokenSecretName,
			Namespace: ServiceAccountNamespace,
		},
		Data: map[string][]byte{
			"token": []byte("test-token-value"),
		},
	}
	client := fake.NewSimpleClientset(secret)
	s := NewSetup(client, "", false)
	ctx := context.Background()

	token, err := s.getToken(ctx)
	if err != nil {
		t.Fatalf("getToken failed: %v", err)
	}
	if token != "test-token-value" {
		t.Errorf("unexpected token: %s", token)
	}
}

func TestGetTokenMissing(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "", false)
	ctx := context.Background()

	_, err := s.getToken(ctx)
	if err == nil {
		t.Error("expected error when secret doesn't exist")
	}
}

func TestGetTokenEmpty(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      TokenSecretName,
			Namespace: ServiceAccountNamespace,
		},
		Data: map[string][]byte{},
	}
	client := fake.NewSimpleClientset(secret)
	s := NewSetup(client, "", false)
	ctx := context.Background()

	_, err := s.getToken(ctx)
	if err == nil {
		t.Error("expected error when token not in secret")
	}
}

func TestProbeKubeconfigPath(t *testing.T) {
	path := ProbeKubeconfigPath()
	if path != ".kube/probe.yaml" {
		t.Errorf("unexpected path: %s", path)
	}
}

func TestProbeKubeconfigExists(t *testing.T) {
	origDir, _ := os.Getwd()
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	if ProbeKubeconfigExists() {
		t.Error("should return false when file doesn't exist")
	}

	if err := os.MkdirAll(".kube", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".kube/probe.yaml", []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if !ProbeKubeconfigExists() {
		t.Error("should return true when file exists")
	}
}

func TestGenerateKubeconfig(t *testing.T) {
	tmpDir := t.TempDir()

	sourceConfig := `apiVersion: v1
kind: Config
current-context: test-context
clusters:
- name: test-cluster
  cluster:
    server: https://kubernetes.example.com:6443
    certificate-authority-data: dGVzdC1jYS1kYXRh
contexts:
- name: test-context
  context:
    cluster: test-cluster
    user: test-user
users:
- name: test-user
  user:
    token: old-token
`
	sourcePath := filepath.Join(tmpDir, "source-config")
	if err := os.WriteFile(sourcePath, []byte(sourceConfig), 0644); err != nil {
		t.Fatal(err)
	}

	client := fake.NewSimpleClientset()
	s := NewSetup(client, sourcePath, false)
	ctx := context.Background()

	outputPath := filepath.Join(tmpDir, "output", "probe.yaml")
	if err := s.generateKubeconfig(ctx, outputPath, "new-test-token"); err != nil {
		t.Fatalf("generateKubeconfig failed: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output: %v", err)
	}

	content := string(data)
	if len(content) == 0 {
		t.Error("output file is empty")
	}

	info, _ := os.Stat(outputPath)
	if info.Mode().Perm() != 0600 {
		t.Errorf("unexpected permissions: %o", info.Mode().Perm())
	}
}

func TestGenerateKubeconfigInvalidSource(t *testing.T) {
	client := fake.NewSimpleClientset()
	s := NewSetup(client, "/nonexistent/config", false)
	ctx := context.Background()

	err := s.generateKubeconfig(ctx, "/tmp/output", "token")
	if err == nil {
		t.Error("expected error for invalid source kubeconfig")
	}
}

func TestConstants(t *testing.T) {
	if ServiceAccountName != "cluster-reader" {
		t.Errorf("unexpected ServiceAccountName: %s", ServiceAccountName)
	}
	if ServiceAccountNamespace != "default" {
		t.Errorf("unexpected ServiceAccountNamespace: %s", ServiceAccountNamespace)
	}
	if ClusterRoleName != "cluster-reader-no-secrets" {
		t.Errorf("unexpected ClusterRoleName: %s", ClusterRoleName)
	}
	if ClusterRoleBindingName != "cluster-reader-binding" {
		t.Errorf("unexpected ClusterRoleBindingName: %s", ClusterRoleBindingName)
	}
	if TokenSecretName != "cluster-reader-token" {
		t.Errorf("unexpected TokenSecretName: %s", TokenSecretName)
	}
}
