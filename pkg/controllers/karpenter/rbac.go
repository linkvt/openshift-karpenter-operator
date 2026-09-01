package karpenter

import (
	rbacv1 "k8s.io/api/rbac/v1"
	rbacac "k8s.io/client-go/applyconfigurations/rbac/v1"
)

func policyRules(rules []rbacv1.PolicyRule) []*rbacac.PolicyRuleApplyConfiguration {
	out := make([]*rbacac.PolicyRuleApplyConfiguration, len(rules))
	for i, r := range rules {
		out[i] = rbacac.PolicyRule().
			WithVerbs(r.Verbs...).
			WithAPIGroups(r.APIGroups...).
			WithResources(r.Resources...).
			WithResourceNames(r.ResourceNames...).
			WithNonResourceURLs(r.NonResourceURLs...)
	}
	return out
}

func roleRef(ref rbacv1.RoleRef) *rbacac.RoleRefApplyConfiguration {
	return rbacac.RoleRef().
		WithAPIGroup(ref.APIGroup).
		WithKind(ref.Kind).
		WithName(ref.Name)
}

func subjects(subs []rbacv1.Subject, ns string) []*rbacac.SubjectApplyConfiguration {
	out := make([]*rbacac.SubjectApplyConfiguration, len(subs))
	for i, s := range subs {
		sub := rbacac.Subject().
			WithKind(s.Kind).
			WithName(s.Name).
			WithAPIGroup(s.APIGroup)
		if s.Kind == rbacv1.ServiceAccountKind {
			if s.Namespace != "" {
				sub.WithNamespace(s.Namespace)
			} else {
				sub.WithNamespace(ns)
			}
		}
		out[i] = sub
	}
	return out
}
