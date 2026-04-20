package matchers

import (
	"fmt"

	"github.com/onsi/gomega/matchers"
	"github.com/onsi/gomega/types"
	"github.com/openmcp-project/controller-utils/pkg/collections"
	"sigs.k8s.io/yaml"

	rbacv1 "k8s.io/api/rbac/v1"

	"k8s.io/apimachinery/pkg/util/sets"
)

// MatchPolicyRules returns a GomegaMatcher that matches a slice of rbacv1.PolicyRule by comparing the content of the fields, ignoring the order of items in slices and the order of rules in the slice.
func MatchPolicyRules(rules []rbacv1.PolicyRule) types.GomegaMatcher {
	res := &policyRulesMatcher{}
	if rules != nil {
		res.com = &matchers.ConsistOfMatcher{Elements: []any{rules}}
	}
	return res
}

type policyRulesMatcher struct {
	com *matchers.ConsistOfMatcher
}

var _ types.GomegaMatcher = &policyRulesMatcher{}

// Match implements types.GomegaMatcher.
func (prms *policyRulesMatcher) Match(actualRaw any) (success bool, err error) {
	if actualRaw == nil {
		return prms.com == nil, nil
	}
	if prms.com == nil {
		return false, nil
	}
	actual, ok := actualRaw.([]rbacv1.PolicyRule)
	if !ok {
		return false, fmt.Errorf("expected actual to be of type []rbacv1.PolicyRule, got %T", actualRaw)
	}

	actualRuleMatchers := collections.ProjectSliceToSlice(actual, func(rule rbacv1.PolicyRule) types.GomegaMatcher {
		return MatchPolicyRule(rule)
	})
	return prms.com.Match(actualRuleMatchers)
}

// FailureMessage implements types.GomegaMatcher.
func (prms *policyRulesMatcher) FailureMessage(actual any) (message string) {
	return prms.com.FailureMessage(actual)
}

// NegatedFailureMessage implements types.GomegaMatcher.
func (prms *policyRulesMatcher) NegatedFailureMessage(actual any) (message string) {
	return prms.com.NegatedFailureMessage(actual)
}

// MatchPolicyRule returns a GomegaMatcher that matches a rbacv1.PolicyRule by comparing the content of the fields, ignoring the order of items in slices.
// It can take either a rbacv1.PolicyRule or a pointer to rbacv1.PolicyRule as actual value.
func MatchPolicyRule(rule any) types.GomegaMatcher {
	switch parsed := rule.(type) {
	case rbacv1.PolicyRule:
		return &policyRuleMatcher{expected: &parsed}
	case *rbacv1.PolicyRule:
		return &policyRuleMatcher{expected: parsed}
	default:
		return &policyRuleMatcher{invalid: fmt.Errorf("MatchPolicyRule matcher expects a rbacv1.PolicyRule or *rbacv1.PolicyRule as argument, got %T", rule)}
	}
}

type policyRuleMatcher struct {
	invalid  error
	expected *rbacv1.PolicyRule
}

func (prm *policyRuleMatcher) GomegaString() string {
	if prm == nil {
		return "<nil>"
	}
	if prm.invalid != nil {
		return fmt.Sprintf("<invalid matcher: %v>", prm.invalid.Error())
	}
	return stringifyPolicyRule(prm.expected)
}

var _ types.GomegaMatcher = &policyRuleMatcher{}

// Match implements types.GomegaMatcher.
func (prm *policyRuleMatcher) Match(actualRaw any) (success bool, err error) {
	if prm.invalid != nil {
		return false, prm.invalid
	}
	if actualRaw == nil {
		return prm.expected == nil, nil
	}
	if prm.expected == nil {
		return false, nil
	}
	var rule rbacv1.PolicyRule
	switch actual := actualRaw.(type) {
	case *rbacv1.PolicyRule:
		rule = *actual
	case rbacv1.PolicyRule:
		rule = actual
	default:
		return false, fmt.Errorf("expected actual (or &actual) to be of type *rbacv1.PolicyRule, got %T", actualRaw)
	}

	expectedApiGroups := sets.New(prm.expected.APIGroups...)
	actualApiGroups := sets.New(rule.APIGroups...)
	if !expectedApiGroups.Equal(actualApiGroups) {
		return false, nil
	}

	expectedNonResourceURLs := sets.New(prm.expected.NonResourceURLs...)
	actualNonResourceURLs := sets.New(rule.NonResourceURLs...)
	if !expectedNonResourceURLs.Equal(actualNonResourceURLs) {
		return false, nil
	}

	expectedResources := sets.New(prm.expected.Resources...)
	actualResources := sets.New(rule.Resources...)
	if !expectedResources.Equal(actualResources) {
		return false, nil
	}

	expectedResourceNames := sets.New(prm.expected.ResourceNames...)
	actualResourceNames := sets.New(rule.ResourceNames...)
	if !expectedResourceNames.Equal(actualResourceNames) {
		return false, nil
	}

	expectedVerbs := sets.New(prm.expected.Verbs...)
	actualVerbs := sets.New(rule.Verbs...)
	if !expectedVerbs.Equal(actualVerbs) {
		return false, nil
	}

	return true, nil
}

// FailureMessage implements types.GomegaMatcher.
func (prm *policyRuleMatcher) FailureMessage(actual any) (message string) {
	var expectedStr string
	if prm.invalid != nil {
		expectedStr = fmt.Sprintf("<invalid matcher: %v>", prm.invalid.Error())
	} else {
		expectedStr = stringifyPolicyRule(prm.expected)
	}
	return fmt.Sprintf("Expected\n\t%#v\nto equal \n\t%#v", stringifyPolicyRule(actual.(*rbacv1.PolicyRule)), expectedStr)
}

// NegatedFailureMessage implements types.GomegaMatcher.
func (prm *policyRuleMatcher) NegatedFailureMessage(actual any) (message string) {
	var expectedStr string
	if prm.invalid != nil {
		expectedStr = fmt.Sprintf("<invalid matcher: %v>", prm.invalid.Error())
	} else {
		expectedStr = stringifyPolicyRule(prm.expected)
	}
	return fmt.Sprintf("Expected\n\t%#v\nto not equal \n\t%#v", stringifyPolicyRule(actual.(*rbacv1.PolicyRule)), expectedStr)
}

func stringifyPolicyRule(rule *rbacv1.PolicyRule) string {
	if rule == nil {
		return "<nil>"
	}
	sortedRule := rbacv1.PolicyRule{}
	if rule.APIGroups != nil {
		sortedRule.APIGroups = sets.List(sets.New(rule.APIGroups...))
	}
	if rule.NonResourceURLs != nil {
		sortedRule.NonResourceURLs = sets.List(sets.New(rule.NonResourceURLs...))
	}
	if rule.Resources != nil {
		sortedRule.Resources = sets.List(sets.New(rule.Resources...))
	}
	if rule.ResourceNames != nil {
		sortedRule.ResourceNames = sets.List(sets.New(rule.ResourceNames...))
	}
	if rule.Verbs != nil {
		sortedRule.Verbs = sets.List(sets.New(rule.Verbs...))
	}

	data, err := yaml.Marshal(sortedRule)
	if err != nil {
		return fmt.Sprintf("<error marshalling policy rule: %v>", err)
	}
	return string(data)
}
