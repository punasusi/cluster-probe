package checks

import (
	"context"
	"fmt"
	"strings"

	"github.com/punasusi/cluster-probe/pkg/probe"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type RBACAudit struct{}

func NewRBACAudit() *RBACAudit {
	return &RBACAudit{}
}

func (c *RBACAudit) Name() string {
	return "rbac-audit"
}

func (c *RBACAudit) Tier() int {
	return 5
}

func (c *RBACAudit) Run(ctx context.Context, client kubernetes.Interface) (*probe.CheckResult, error) {
	result := &probe.CheckResult{
		Name:		c.Name(),
		Tier:		c.Tier(),
		Results:	[]probe.Result{},
	}

	clusterRoles, err := client.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster roles: %w", err)
	}

	wildcardRoles := 0
	secretAccessRoles := 0
	adminRoles := 0

	for _, cr := range clusterRoles.Items {

		if strings.HasPrefix(cr.Name, "system:") {
			continue
		}

		issues := c.analyzeRules(cr.Rules)

		if issues.hasWildcardAll {
			wildcardRoles++
			result.Results = append(result.Results, probe.Result{
				CheckName:	c.Name(),
				Severity:	probe.SeverityWarning,
				Message:	fmt.Sprintf("ClusterRole %s has wildcard access to all resources", cr.Name),
				Details: []string{
					"Rule: apiGroups: ['*'], resources: ['*'], verbs: ['*']",
				},
				Remediation:	"Consider limiting to specific resources and verbs following least-privilege",
			})
		}

		if issues.hasSecretAccess {
			secretAccessRoles++
		}

		if issues.hasAdminVerbs {
			adminRoles++
		}
	}

	clusterRoleBindings, err := client.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list cluster role bindings: %w", err)
	}

	dangerousBindings := 0
	for _, crb := range clusterRoleBindings.Items {

		if strings.HasPrefix(crb.Name, "system:") {
			continue
		}

		if crb.RoleRef.Name == "cluster-admin" {
			for _, subject := range crb.Subjects {

				if subject.Kind == "ServiceAccount" && subject.Namespace != "kube-system" {
					dangerousBindings++
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("ServiceAccount %s/%s has cluster-admin access", subject.Namespace, subject.Name),
						Details: []string{
							fmt.Sprintf("Binding: %s", crb.Name),
						},
						Remediation:	"Review if cluster-admin is necessary; consider a more restrictive role",
					})
				}

				if subject.Kind == "Group" && !strings.HasPrefix(subject.Name, "system:") {
					dangerousBindings++
					result.Results = append(result.Results, probe.Result{
						CheckName:	c.Name(),
						Severity:	probe.SeverityWarning,
						Message:	fmt.Sprintf("Group %s has cluster-admin access", subject.Name),
						Details: []string{
							fmt.Sprintf("Binding: %s", crb.Name),
						},
						Remediation:	"Review group membership and consider more granular RBAC",
					})
				}
			}
		}
	}

	roles, err := client.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, role := range roles.Items {
			issues := c.analyzeRules(role.Rules)
			if issues.hasWildcardAll {
				result.Results = append(result.Results, probe.Result{
					CheckName:	c.Name(),
					Severity:	probe.SeverityWarning,
					Message:	fmt.Sprintf("Role %s/%s has wildcard access", role.Namespace, role.Name),
					Remediation:	"Consider limiting to specific resources and verbs",
				})
			}
		}
	}

	severity := probe.SeverityOK
	if wildcardRoles > 0 || dangerousBindings > 0 {
		severity = probe.SeverityWarning
	}

	totalClusterRoles := 0
	for _, cr := range clusterRoles.Items {
		if !strings.HasPrefix(cr.Name, "system:") {
			totalClusterRoles++
		}
	}

	result.Results = append(result.Results, probe.Result{
		CheckName:	c.Name(),
		Severity:	severity,
		Message:	"RBAC audit summary",
		Details: []string{
			fmt.Sprintf("Custom ClusterRoles: %d", totalClusterRoles),
			fmt.Sprintf("Wildcard access roles: %d", wildcardRoles),
			fmt.Sprintf("Roles with secret access: %d", secretAccessRoles),
			fmt.Sprintf("Dangerous bindings: %d", dangerousBindings),
		},
	})

	return result, nil
}

type ruleIssues struct {
	hasWildcardAll	bool
	hasSecretAccess	bool
	hasAdminVerbs	bool
}

func (c *RBACAudit) analyzeRules(rules []rbacv1.PolicyRule) ruleIssues {
	issues := ruleIssues{}

	for _, rule := range rules {

		if containsString(rule.APIGroups, "*") &&
			containsString(rule.Resources, "*") &&
			containsString(rule.Verbs, "*") {
			issues.hasWildcardAll = true
		}

		if (containsString(rule.APIGroups, "") || containsString(rule.APIGroups, "*")) &&
			(containsString(rule.Resources, "secrets") || containsString(rule.Resources, "*")) {
			issues.hasSecretAccess = true
		}

		adminVerbs := []string{"create", "delete", "deletecollection", "patch", "update", "*"}
		for _, verb := range rule.Verbs {
			if containsString(adminVerbs, verb) {
				issues.hasAdminVerbs = true
				break
			}
		}
	}

	return issues
}

func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}
