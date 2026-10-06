package clusteroperator

import (
	"testing"
	"time"

	autoscalingv1alpha1 "github.com/openshift/karpenter-operator/pkg/apis/autoscaling/v1alpha1"

	configv1 "github.com/openshift/api/config/v1"
	configac "github.com/openshift/client-go/config/applyconfigurations/config/v1"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testNamespace      = "openshift-karpenter"
	testReleaseVersion = "4.19.0"
)

var testKarpenterCR = &autoscalingv1alpha1.Karpenter{
	ObjectMeta: metav1.ObjectMeta{Name: autoscalingv1alpha1.SingletonName},
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = appsv1.AddToScheme(s)
	_ = configv1.Install(s)
	_ = autoscalingv1alpha1.AddToScheme(s)
	return s
}

func newTestController(cfg *ControllerConfig, objs ...client.Object) *Controller {
	return &Controller{
		client: fakeclient.NewClientBuilder().
			WithScheme(testScheme()).
			WithObjects(objs...).
			WithStatusSubresource(&configv1.ClusterOperator{}).
			Build(),
		config: cfg,
	}
}

func TestReconcile(t *testing.T) {
	testCases := map[string]struct {
		objs                []client.Object
		unsupportedPlatform *configv1.PlatformType
		expectAvailable     configv1.ConditionStatus
		expectProgressing   configv1.ConditionStatus
		expectDegraded      configv1.ConditionStatus
		expectUpgradeable   configv1.ConditionStatus
		expectMessageOn     configv1.ClusterStatusConditionType
		expectMessage       string
		expectRelatedObjCt  int
	}{
		"When no Karpenter CR exists, it should report Available": {
			objs:               nil,
			expectAvailable:    configv1.ConditionTrue,
			expectProgressing:  configv1.ConditionFalse,
			expectDegraded:     configv1.ConditionFalse,
			expectUpgradeable:  configv1.ConditionTrue,
			expectMessageOn:    configv1.OperatorAvailable,
			expectMessage:      "at version " + testReleaseVersion,
			expectRelatedObjCt: 6,
		},
		"When the operand Deployment does not exist, it should report Progressing": {
			objs:               []client.Object{testKarpenterCR},
			expectAvailable:    configv1.ConditionTrue,
			expectProgressing:  configv1.ConditionTrue,
			expectDegraded:     configv1.ConditionFalse,
			expectUpgradeable:  configv1.ConditionTrue,
			expectMessageOn:    configv1.OperatorProgressing,
			expectMessage:      "Waiting for karpenter Deployment to be created",
			expectRelatedObjCt: 6,
		},
		"When the operand Deployment is not ready, it should report Progressing": {
			objs: []client.Object{
				testKarpenterCR,
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter", Namespace: testNamespace},
					Status:     appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 0},
				},
			},
			expectAvailable:    configv1.ConditionTrue,
			expectProgressing:  configv1.ConditionTrue,
			expectDegraded:     configv1.ConditionFalse,
			expectUpgradeable:  configv1.ConditionTrue,
			expectMessageOn:    configv1.OperatorProgressing,
			expectMessage:      "Waiting for karpenter Deployment to become available",
			expectRelatedObjCt: 6,
		},
		"When the operand Deployment is rolling out, it should report Progressing": {
			objs: []client.Object{
				testKarpenterCR,
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter", Namespace: testNamespace},
					Status:     appsv1.DeploymentStatus{Replicas: 2, AvailableReplicas: 1, UpdatedReplicas: 1},
				},
			},
			expectAvailable:    configv1.ConditionTrue,
			expectProgressing:  configv1.ConditionTrue,
			expectDegraded:     configv1.ConditionFalse,
			expectUpgradeable:  configv1.ConditionTrue,
			expectMessageOn:    configv1.OperatorProgressing,
			expectMessage:      "Karpenter Deployment is rolling out",
			expectRelatedObjCt: 6,
		},
		"When the operand Deployment is healthy, it should report Available": {
			objs: []client.Object{
				testKarpenterCR,
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{Name: "karpenter", Namespace: testNamespace},
					Status:     appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1},
				},
			},
			expectAvailable:    configv1.ConditionTrue,
			expectProgressing:  configv1.ConditionFalse,
			expectDegraded:     configv1.ConditionFalse,
			expectUpgradeable:  configv1.ConditionTrue,
			expectMessageOn:    configv1.OperatorAvailable,
			expectMessage:      "at version " + testReleaseVersion,
			expectRelatedObjCt: 6,
		},
		"When the platform is unsupported, it should report Available even if a Karpenter CR exists": {
			objs:                []client.Object{testKarpenterCR},
			unsupportedPlatform: new(configv1.VSpherePlatformType),
			expectAvailable:     configv1.ConditionTrue,
			expectProgressing:   configv1.ConditionFalse,
			expectDegraded:      configv1.ConditionFalse,
			expectUpgradeable:   configv1.ConditionTrue,
			expectMessageOn:     configv1.OperatorAvailable,
			expectMessage:       "Karpenter is not supported on platform VSphere, no operand is deployed",
			expectRelatedObjCt:  4,
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			sc := newTestController(&ControllerConfig{
				Namespace:           testNamespace,
				ReleaseVersion:      testReleaseVersion,
				UnsupportedPlatform: tc.unsupportedPlatform,
			}, tc.objs...)

			if _, err := sc.Reconcile(t.Context(), ctrl.Request{}); err != nil {
				t.Fatalf("Reconcile() returned error: %v", err)
			}

			co := &configv1.ClusterOperator{}
			if err := sc.client.Get(t.Context(), client.ObjectKey{Name: clusterOperatorName}, co); err != nil {
				t.Fatalf("failed to get ClusterOperator: %v", err)
			}

			assertCondition(t, co, configv1.OperatorAvailable, tc.expectAvailable)
			assertCondition(t, co, configv1.OperatorProgressing, tc.expectProgressing)
			assertCondition(t, co, configv1.OperatorDegraded, tc.expectDegraded)
			assertCondition(t, co, configv1.OperatorUpgradeable, tc.expectUpgradeable)

			if cond := findCondition(co.Status.Conditions, tc.expectMessageOn); cond == nil {
				t.Errorf("condition %s not found for message check", tc.expectMessageOn)
			} else if cond.Message != tc.expectMessage {
				t.Errorf("expected %s message %q, got %q", tc.expectMessageOn, tc.expectMessage, cond.Message)
			}

			if len(co.Status.Versions) != 1 || co.Status.Versions[0].Version != testReleaseVersion {
				t.Errorf("expected version %q, got %+v", testReleaseVersion, co.Status.Versions)
			}

			if len(co.Status.RelatedObjects) != tc.expectRelatedObjCt {
				t.Errorf("expected %d related objects, got %d", tc.expectRelatedObjCt, len(co.Status.RelatedObjects))
			}
		})
	}
}

func TestReconcilePreservesTransitionTimes(t *testing.T) {
	seeded := metav1.NewTime(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	sc := newTestController(
		&ControllerConfig{Namespace: testNamespace, ReleaseVersion: testReleaseVersion},
		testKarpenterCR,
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "karpenter", Namespace: testNamespace},
			Status:     appsv1.DeploymentStatus{Replicas: 1, AvailableReplicas: 1, UpdatedReplicas: 1},
		},
		&configv1.ClusterOperator{
			ObjectMeta: metav1.ObjectMeta{Name: clusterOperatorName},
			Status: configv1.ClusterOperatorStatus{
				Conditions: []configv1.ClusterOperatorStatusCondition{
					{
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionFalse,
						LastTransitionTime: seeded,
					},
					{
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionTrue,
						Reason:             "SomePreviousError",
						LastTransitionTime: seeded,
					},
					{
						// Upgradeable=True matches what reconcile will produce,
						// so LastTransitionTime must be preserved.
						Type:               configv1.OperatorUpgradeable,
						Status:             configv1.ConditionTrue,
						Reason:             "AsExpected",
						LastTransitionTime: seeded,
					},
				},
			},
		},
	)

	if _, err := sc.Reconcile(t.Context(), ctrl.Request{}); err != nil {
		t.Fatalf("Reconcile() returned error: %v", err)
	}

	co := &configv1.ClusterOperator{}
	if err := sc.client.Get(t.Context(), client.ObjectKey{Name: clusterOperatorName}, co); err != nil {
		t.Fatalf("failed to get ClusterOperator: %v", err)
	}

	// Available changed False→True: timestamp must advance.
	available := findCondition(co.Status.Conditions, configv1.OperatorAvailable)
	if available == nil {
		t.Fatalf("condition %s not found", configv1.OperatorAvailable)
	}
	if !available.LastTransitionTime.After(seeded.Time) {
		t.Errorf("Available changed status but LastTransitionTime was not updated: got %v", available.LastTransitionTime)
	}

	// Upgradeable stayed True→True: timestamp must be preserved.
	upgradeable := findCondition(co.Status.Conditions, configv1.OperatorUpgradeable)
	if upgradeable == nil {
		t.Fatalf("condition %s not found", configv1.OperatorUpgradeable)
	}
	if !upgradeable.LastTransitionTime.Equal(&seeded) {
		t.Errorf("Upgradeable status unchanged but LastTransitionTime changed: got %v, want %v", upgradeable.LastTransitionTime, seeded)
	}
}

func TestConditionHelpers(t *testing.T) {
	type expectedCondition struct {
		status configv1.ConditionStatus
		reason string
	}

	testCases := map[string]struct {
		conditions []*configac.ClusterOperatorStatusConditionApplyConfiguration
		expect     map[configv1.ClusterStatusConditionType]expectedCondition
	}{
		"When building available conditions, it should set Available with the given reason": {
			conditions: availableConditions("KarpenterNotFound", "all good"),
			expect: map[configv1.ClusterStatusConditionType]expectedCondition{
				configv1.OperatorAvailable:   {configv1.ConditionTrue, "KarpenterNotFound"},
				configv1.OperatorProgressing: {configv1.ConditionFalse, "AsExpected"},
				configv1.OperatorDegraded:    {configv1.ConditionFalse, "AsExpected"},
				configv1.OperatorUpgradeable: {configv1.ConditionTrue, "AsExpected"},
			},
		},
		"When building progressing conditions, it should set Progressing with the given reason": {
			conditions: progressingConditions("Rolling", "rolling out"),
			expect: map[configv1.ClusterStatusConditionType]expectedCondition{
				configv1.OperatorAvailable:   {configv1.ConditionTrue, "AsExpected"},
				configv1.OperatorProgressing: {configv1.ConditionTrue, "Rolling"},
				configv1.OperatorDegraded:    {configv1.ConditionFalse, "AsExpected"},
				configv1.OperatorUpgradeable: {configv1.ConditionTrue, "AsExpected"},
			},
		},
		"When building degraded conditions, it should set Degraded with the given reason": {
			conditions: degradedConditions("Broken", "something failed"),
			expect: map[configv1.ClusterStatusConditionType]expectedCondition{
				configv1.OperatorAvailable:   {configv1.ConditionTrue, "AsExpected"},
				configv1.OperatorProgressing: {configv1.ConditionFalse, "AsExpected"},
				configv1.OperatorDegraded:    {configv1.ConditionTrue, "Broken"},
				configv1.OperatorUpgradeable: {configv1.ConditionTrue, "AsExpected"},
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			for _, c := range tc.conditions {
				condType := *c.Type
				want, ok := tc.expect[condType]
				if !ok {
					t.Fatalf("unexpected condition type: %s", condType)
				}
				if *c.Status != want.status {
					t.Errorf("%s status: got %s, want %s", condType, *c.Status, want.status)
				}
				if *c.Reason != want.reason {
					t.Errorf("%s reason: got %q, want %q", condType, *c.Reason, want.reason)
				}
			}
		})
	}
}

func assertCondition(t *testing.T, co *configv1.ClusterOperator, condType configv1.ClusterStatusConditionType, expected configv1.ConditionStatus) {
	t.Helper()
	cond := findCondition(co.Status.Conditions, condType)
	if cond == nil {
		t.Errorf("condition %s not found", condType)
		return
	}
	if cond.Status != expected {
		t.Errorf("condition %s: got %s, want %s", condType, cond.Status, expected)
	}
}
