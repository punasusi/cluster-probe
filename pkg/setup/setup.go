package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	ServiceAccountName	= "cluster-reader"
	ServiceAccountNamespace	= "default"
	ClusterRoleName		= "cluster-reader-no-secrets"
	ClusterRoleBindingName	= "cluster-reader-binding"
	TokenSecretName		= "cluster-reader-token"
)

type Setup struct {
	client		kubernetes.Interface
	verbose		bool
	kubeconfigPath	string
}

func NewSetup(client kubernetes.Interface, kubeconfigPath string, verbose bool) *Setup {
	return &Setup{
		client:		client,
		verbose:	verbose,
		kubeconfigPath:	kubeconfigPath,
	}
}

func (s *Setup) log(format string, args ...interface{}) {
	if s.verbose {
		fmt.Fprintf(os.Stderr, "[setup] "+format+"\n", args...)
	}
}

func (s *Setup) Run(ctx context.Context, outputPath string) error {
	s.log("Creating read-only service account...")

	if err := s.createServiceAccount(ctx); err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}

	crdGroups, err := s.getCRDAPIGroups(ctx)
	if err != nil {
		s.log("Warning: could not get CRD API groups: %v", err)
		crdGroups = []string{}
	}

	if err := s.createClusterRole(ctx, crdGroups); err != nil {
		return fmt.Errorf("failed to create cluster role: %w", err)
	}

	if err := s.createClusterRoleBinding(ctx); err != nil {
		return fmt.Errorf("failed to create cluster role binding: %w", err)
	}

	if err := s.createTokenSecret(ctx); err != nil {
		return fmt.Errorf("failed to create token secret: %w", err)
	}

	token, err := s.getToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get token: %w", err)
	}

	if err := s.generateKubeconfig(ctx, outputPath, token); err != nil {
		return fmt.Errorf("failed to generate kubeconfig: %w", err)
	}

	s.log("Setup complete! Kubeconfig written to %s", outputPath)
	return nil
}

func (s *Setup) createServiceAccount(ctx context.Context) error {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:		ServiceAccountName,
			Namespace:	ServiceAccountNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":	"cluster-probe",
				"app.kubernetes.io/managed-by":	"cluster-probe",
			},
		},
	}

	_, err := s.client.CoreV1().ServiceAccounts(ServiceAccountNamespace).Create(ctx, sa, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			s.log("ServiceAccount already exists")
			return nil
		}
		return err
	}

	s.log("Created ServiceAccount %s/%s", ServiceAccountNamespace, ServiceAccountName)
	return nil
}

func (s *Setup) getCRDAPIGroups(ctx context.Context) ([]string, error) {

	config, err := clientcmd.BuildConfigFromFlags("", s.kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	apiextClient, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create apiextensions client: %w", err)
	}

	crds, err := apiextClient.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {

		return nil, fmt.Errorf("failed to list CRDs: %w", err)
	}

	groupSet := make(map[string]struct{})
	for _, crd := range crds.Items {
		if crd.Spec.Group != "" {
			groupSet[crd.Spec.Group] = struct{}{}
		}
	}

	groups := make([]string, 0, len(groupSet))
	for group := range groupSet {
		groups = append(groups, group)
	}

	s.log("Found %d CRD API groups", len(groups))
	return groups, nil
}

func (s *Setup) createClusterRole(ctx context.Context, crdGroups []string) error {

	rules := []rbacv1.PolicyRule{

		{
			APIGroups:	[]string{""},
			Resources: []string{
				"bindings",
				"componentstatuses",
				"configmaps",
				"endpoints",
				"events",
				"limitranges",
				"namespaces",
				"nodes",
				"persistentvolumeclaims",
				"persistentvolumes",
				"pods",
				"pods/log",
				"pods/status",
				"podtemplates",
				"replicationcontrollers",
				"resourcequotas",
				"serviceaccounts",
				"services",
			},
			Verbs:	[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"apps"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"extensions"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"batch"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"networking.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"rbac.authorization.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"storage.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"apiregistration.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"apiextensions.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"metrics.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"autoscaling"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"policy"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"coordination.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"node.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"admissionregistration.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"scheduling.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"certificates.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"discovery.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"events.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},

		{
			APIGroups:	[]string{"flowcontrol.apiserver.k8s.io"},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		},
	}

	for _, group := range crdGroups {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups:	[]string{group},
			Resources:	[]string{"*"},
			Verbs:		[]string{"get", "list", "watch"},
		})
	}

	role := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:	ClusterRoleName,
			Labels: map[string]string{
				"app.kubernetes.io/name":	"cluster-probe",
				"app.kubernetes.io/managed-by":	"cluster-probe",
			},
		},
		Rules:	rules,
	}

	_, err := s.client.RbacV1().ClusterRoles().Create(ctx, role, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {

			existing, getErr := s.client.RbacV1().ClusterRoles().Get(ctx, ClusterRoleName, metav1.GetOptions{})
			if getErr != nil {
				return getErr
			}
			existing.Rules = rules
			_, updateErr := s.client.RbacV1().ClusterRoles().Update(ctx, existing, metav1.UpdateOptions{})
			if updateErr != nil {
				return updateErr
			}
			s.log("Updated ClusterRole %s with %d rules", ClusterRoleName, len(rules))
			return nil
		}
		return err
	}

	s.log("Created ClusterRole %s with %d rules", ClusterRoleName, len(rules))
	return nil
}

func (s *Setup) createClusterRoleBinding(ctx context.Context) error {
	binding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:	ClusterRoleBindingName,
			Labels: map[string]string{
				"app.kubernetes.io/name":	"cluster-probe",
				"app.kubernetes.io/managed-by":	"cluster-probe",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup:	"rbac.authorization.k8s.io",
			Kind:		"ClusterRole",
			Name:		ClusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:		"ServiceAccount",
				Name:		ServiceAccountName,
				Namespace:	ServiceAccountNamespace,
			},
		},
	}

	_, err := s.client.RbacV1().ClusterRoleBindings().Create(ctx, binding, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			s.log("ClusterRoleBinding already exists")
			return nil
		}
		return err
	}

	s.log("Created ClusterRoleBinding %s", ClusterRoleBindingName)
	return nil
}

func (s *Setup) createTokenSecret(ctx context.Context) error {

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:		TokenSecretName,
			Namespace:	ServiceAccountNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":	"cluster-probe",
				"app.kubernetes.io/managed-by":	"cluster-probe",
			},
			Annotations: map[string]string{
				"kubernetes.io/service-account.name": ServiceAccountName,
			},
		},
		Type:	corev1.SecretTypeServiceAccountToken,
	}

	_, err := s.client.CoreV1().Secrets(ServiceAccountNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			s.log("Token Secret already exists")
			return nil
		}
		return err
	}

	s.log("Created token Secret %s/%s", ServiceAccountNamespace, TokenSecretName)
	return nil
}

func (s *Setup) getToken(ctx context.Context) (string, error) {

	secret, err := s.client.CoreV1().Secrets(ServiceAccountNamespace).Get(ctx, TokenSecretName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to get token secret: %w", err)
	}

	token, ok := secret.Data["token"]
	if !ok || len(token) == 0 {
		return "", fmt.Errorf("token not yet available in secret")
	}

	s.log("Retrieved service account token")
	return string(token), nil
}

func (s *Setup) generateKubeconfig(ctx context.Context, outputPath string, token string) error {

	config, err := clientcmd.LoadFromFile(s.kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to load source kubeconfig: %w", err)
	}

	currentContext := config.CurrentContext
	if currentContext == "" {
		return fmt.Errorf("no current context in source kubeconfig")
	}

	contextInfo, ok := config.Contexts[currentContext]
	if !ok {
		return fmt.Errorf("context %s not found in source kubeconfig", currentContext)
	}

	clusterInfo, ok := config.Clusters[contextInfo.Cluster]
	if !ok {
		return fmt.Errorf("cluster %s not found in source kubeconfig", contextInfo.Cluster)
	}

	newConfig := clientcmdapi.NewConfig()

	newConfig.Clusters["cluster-probe"] = &clientcmdapi.Cluster{
		Server:				clusterInfo.Server,
		CertificateAuthorityData:	clusterInfo.CertificateAuthorityData,
		CertificateAuthority:		clusterInfo.CertificateAuthority,
		InsecureSkipTLSVerify:		clusterInfo.InsecureSkipTLSVerify,
	}

	newConfig.AuthInfos["cluster-reader"] = &clientcmdapi.AuthInfo{
		Token: token,
	}

	newConfig.Contexts["cluster-probe"] = &clientcmdapi.Context{
		Cluster:	"cluster-probe",
		AuthInfo:	"cluster-reader",
	}

	newConfig.CurrentContext = "cluster-probe"

	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	if err := clientcmd.WriteToFile(*newConfig, outputPath); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	if err := os.Chmod(outputPath, 0600); err != nil {
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	s.log("Generated kubeconfig at %s", outputPath)
	return nil
}

func ProbeKubeconfigPath() string {
	return ".kube/probe.yaml"
}

func ProbeKubeconfigExists() bool {
	_, err := os.Stat(ProbeKubeconfigPath())
	return err == nil
}

var _ = apiextensionsv1.CustomResourceDefinition{}
