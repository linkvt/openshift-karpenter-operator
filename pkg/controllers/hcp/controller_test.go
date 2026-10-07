package hcp

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift/karpenter-operator/pkg/hypershift"

	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestKarpenterDeletion(t *testing.T) {
	scheme := testScheme(t)
	now := time.Now()

	const testNamespace = "test-namespace"

	testCases := []struct {
		name                                string
		hcp                                 *hyperv1beta1.HostedControlPlane
		managementObjects                   []client.Object
		objects                             []client.Object
		expectedNodePools                   int
		expectedNodeClaims                  int
		eventuallyKarpenterFinalizerRemoved bool
		// expectedTerminationAnnotations maps NodeClaim name to whether it should have the termination timestamp annotation
		expectedTerminationAnnotations map[string]bool
	}{
		{
			name: "When HostedControlPlane is deleted with no resources, it should remove the Karpenter finalizer",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					DeletionTimestamp: &metav1.Time{
						Time: now,
					},
					Finalizers: []string{
						karpenterFinalizer,
						"some-other-finalizer",
					},
				},
			},
			objects:                             []client.Object{},
			expectedNodePools:                   0,
			expectedNodeClaims:                  0,
			eventuallyKarpenterFinalizerRemoved: true,
		},
		{
			name: "When HostedControlPlane is deleted, it should delete Karpenter NodePools and remove the Karpenter finalizer",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					DeletionTimestamp: &metav1.Time{
						Time: now,
					},
					Finalizers: []string{
						karpenterFinalizer,
						"some-other-finalizer",
					},
				},
			},
			objects: []client.Object{
				&karpenterv1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool-1",
					},
				},
				&karpenterv1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool-2",
					},
				},
			},
			expectedNodePools:                   0,
			expectedNodeClaims:                  0,
			eventuallyKarpenterFinalizerRemoved: true,
		},
		{
			name: "When HostedControlPlane is deleted with NodePools that cannot be deleted, it should keep the Karpenter finalizer",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					DeletionTimestamp: &metav1.Time{
						Time: now,
					},
					Finalizers: []string{
						karpenterFinalizer,
						"some-other-finalizer",
					},
				},
			},
			objects: func() []client.Object {
				nodepool := &karpenterv1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool-1",
					},
				}
				nodepool.SetFinalizers([]string{"some-finalizer"}) // this prevents the nodepool from being deleted
				return []client.Object{nodepool}
			}(),
			expectedNodePools:                   1,
			expectedNodeClaims:                  0,
			eventuallyKarpenterFinalizerRemoved: false,
		},
		{
			name: "When HostedControlPlane is deleted with NodeClaims pending deletion, it should annotate them and keep the Karpenter finalizer",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					DeletionTimestamp: &metav1.Time{
						Time: now,
					},
					Finalizers: []string{
						karpenterFinalizer,
						"some-other-finalizer",
					},
				},
			},
			objects: []client.Object{
				&karpenterv1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool-1",
					},
				},
				&karpenterv1.NodeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodeclaim-1",
						DeletionTimestamp: &metav1.Time{
							Time: now,
						},
						Finalizers: []string{"karpenter-finalizer"}, // prevents actual deletion
					},
				},
				&karpenterv1.NodeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodeclaim-2",
						DeletionTimestamp: &metav1.Time{
							Time: now,
						},
						Finalizers: []string{"karpenter-finalizer"},
					},
				},
			},
			expectedNodePools:                   0,
			expectedNodeClaims:                  2,
			eventuallyKarpenterFinalizerRemoved: false,
			expectedTerminationAnnotations: map[string]bool{
				"test-nodeclaim-1": true,
				"test-nodeclaim-2": true,
			},
		},
		{
			name: "When NodeClaim already has a termination annotation, it should not set it again",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					DeletionTimestamp: &metav1.Time{
						Time: now,
					},
					Finalizers: []string{
						karpenterFinalizer,
					},
				},
			},
			objects: []client.Object{
				&karpenterv1.NodeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodeclaim-1",
						DeletionTimestamp: &metav1.Time{
							Time: now,
						},
						Finalizers: []string{"karpenter-finalizer"},
						Annotations: map[string]string{
							karpenterv1.NodeClaimTerminationTimestampAnnotationKey: "2024-01-01T00:00:00Z",
						},
					},
				},
			},
			expectedNodePools:                   0,
			expectedNodeClaims:                  1,
			eventuallyKarpenterFinalizerRemoved: false,
			expectedTerminationAnnotations: map[string]bool{
				"test-nodeclaim-1": true, // Already has annotation, should still have it
			},
		},
		{
			name: "When NodeClaim has no deletion timestamp, it should be explicitly deleted and receive a termination annotation",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					DeletionTimestamp: &metav1.Time{
						Time: now,
					},
					Finalizers: []string{
						karpenterFinalizer,
					},
				},
			},
			objects: []client.Object{
				&karpenterv1.NodeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:       "test-nodeclaim-orphaned",
						Finalizers: []string{"karpenter-finalizer"},
						// No DeletionTimestamp - simulates orphaned NodeClaim
					},
				},
			},
			expectedNodePools:                   0,
			expectedNodeClaims:                  1, // Still exists due to finalizer
			eventuallyKarpenterFinalizerRemoved: false,
			expectedTerminationAnnotations: map[string]bool{
				// First reconcile explicitly deletes it (sets DeletionTimestamp),
				// second reconcile sees DeletionTimestamp and sets termination annotation
				"test-nodeclaim-orphaned": true,
			},
		},
		{
			name: "When CAPI Cluster is deleting but HostedControlPlane is not, it should start node cleanup without removing the Karpenter finalizer",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					Finalizers: []string{
						karpenterFinalizer,
						"some-other-finalizer",
					},
				},
				Spec: hyperv1beta1.HostedControlPlaneSpec{
					InfraID: "test-infra-id",
				},
			},
			managementObjects: []client.Object{
				newCAPICluster(testNamespace, "test-infra-id", now),
			},
			objects: []client.Object{
				&karpenterv1.NodePool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-nodepool-1",
					},
				},
			},
			expectedNodePools:                   0,
			expectedNodeClaims:                  0,
			eventuallyKarpenterFinalizerRemoved: false,
		},
		{
			name: "When CAPI Cluster is deleting with NodeClaims, it should clean them up without removing the Karpenter finalizer",
			hcp: &hyperv1beta1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: testNamespace,
					Finalizers: []string{
						karpenterFinalizer,
					},
				},
				Spec: hyperv1beta1.HostedControlPlaneSpec{
					InfraID: "test-infra-id",
				},
			},
			managementObjects: []client.Object{
				newCAPICluster(testNamespace, "test-infra-id", now),
			},
			objects: []client.Object{
				&karpenterv1.NodeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test-nodeclaim-1",
						DeletionTimestamp: &metav1.Time{Time: now},
						Finalizers:        []string{"karpenter-finalizer"},
					},
				},
			},
			expectedNodePools:                   0,
			expectedNodeClaims:                  1,
			eventuallyKarpenterFinalizerRemoved: false,
			expectedTerminationAnnotations: map[string]bool{
				"test-nodeclaim-1": true,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			fakeManagementClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.hcp).
				WithObjects(tc.managementObjects...).
				Build()

			fakeHostedClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objects...).
				Build()

			r := &Controller{
				config:           &ControllerConfig{Namespace: testNamespace},
				managementClient: fakeManagementClient,
				hostedClient:     fakeHostedClient,
			}

			// first reconcile should initiate deletions
			_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(tc.hcp)})
			g.Expect(err).NotTo(HaveOccurred())

			// second reconcile will attempt to remove finalizers
			_, err = r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(tc.hcp)})
			g.Expect(err).NotTo(HaveOccurred())

			// verify HCP finalizers
			hcp, err := hypershift.GetHostedControlPlane(t.Context(), r.managementClient, r.config.Namespace)
			g.Expect(err).NotTo(HaveOccurred())
			if tc.eventuallyKarpenterFinalizerRemoved {
				g.Expect(hcp.Finalizers).NotTo(ContainElement(karpenterFinalizer))
			} else {
				g.Expect(hcp.Finalizers).To(ContainElement(karpenterFinalizer))
			}

			// verify NodePool count
			nodePoolList := &karpenterv1.NodePoolList{}
			err = fakeHostedClient.List(t.Context(), nodePoolList)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(nodePoolList.Items).To(HaveLen(tc.expectedNodePools))

			// verify NodeClaim count
			nodeClaimList := &karpenterv1.NodeClaimList{}
			err = fakeHostedClient.List(t.Context(), nodeClaimList)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(nodeClaimList.Items).To(HaveLen(tc.expectedNodeClaims))

			// verify annotations if specified
			for nodeClaimName, shouldHaveAnnotation := range tc.expectedTerminationAnnotations {
				nodeClaim := &karpenterv1.NodeClaim{}
				err := fakeHostedClient.Get(t.Context(), client.ObjectKey{Name: nodeClaimName}, nodeClaim)
				g.Expect(err).NotTo(HaveOccurred())

				hasAnnotation := nodeClaim.Annotations[karpenterv1.NodeClaimTerminationTimestampAnnotationKey] != ""
				g.Expect(hasAnnotation).To(Equal(shouldHaveAnnotation),
					"NodeClaim %s: expected annotation=%v, got=%v", nodeClaimName, shouldHaveAnnotation, hasAnnotation)
			}
		})
	}
}

func TestReconcileAutoNodeStatus(t *testing.T) {
	const testNamespace = "test-namespace"
	g := NewWithT(t)
	scheme := testScheme(t)
	hcp := &hyperv1beta1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: testNamespace,
		},
	}
	managementClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(hcp).
		WithStatusSubresource(&hyperv1beta1.HostedControlPlane{}).
		Build()
	liveNodeClaim := nodeClaimWithCapacity("nodeclaim-live", "karpenter-node", "8")
	unregisteredNodeClaim := nodeClaimWithCapacity("nodeclaim-unregistered", "missing-node", "16")
	hostedClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "karpenter-node", Labels: map[string]string{karpenterv1.NodePoolLabelKey: "default"}}},
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "other-node"}},
			&liveNodeClaim,
			&unregisteredNodeClaim,
		).
		Build()

	r := &Controller{
		config:           &ControllerConfig{Namespace: testNamespace},
		managementClient: managementClient,
		hostedClient:     hostedClient,
	}
	_, err := r.Reconcile(t.Context(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(hcp)})
	g.Expect(err).NotTo(HaveOccurred())

	updatedHCP := &hyperv1beta1.HostedControlPlane{}
	g.Expect(managementClient.Get(t.Context(), client.ObjectKeyFromObject(hcp), updatedHCP)).To(Succeed())
	g.Expect(updatedHCP.Finalizers).To(ContainElement(karpenterFinalizer))
	g.Expect(updatedHCP.Status.AutoNode.NodeCount).NotTo(BeNil())
	g.Expect(*updatedHCP.Status.AutoNode.NodeCount).To(Equal(int32(1)))
	g.Expect(updatedHCP.Status.AutoNode.NodeClaimCount).NotTo(BeNil())
	g.Expect(*updatedHCP.Status.AutoNode.NodeClaimCount).To(Equal(int32(2)))
	g.Expect(updatedHCP.Status.AutoNode.VCPUs).NotTo(BeNil())
	g.Expect(*updatedHCP.Status.AutoNode.VCPUs).To(Equal(int32(8)))
}

func TestSumNodeClaimVCPUs(t *testing.T) {
	tests := []struct {
		name       string
		nodeClaims []karpenterv1.NodeClaim
		liveNodes  map[string]struct{}
		expected   int32
	}{
		{
			name:       "When there are no NodeClaims, it should return 0",
			nodeClaims: nil,
			liveNodes:  nil,
			expected:   0,
		},
		{
			name: "When NodeClaims have live nodes with capacity, it should sum their CPUs",
			nodeClaims: []karpenterv1.NodeClaim{
				nodeClaimWithCapacity("nc-1", "node-1", "4"),
				nodeClaimWithCapacity("nc-2", "node-2", "8"),
				nodeClaimWithCapacity("nc-3", "node-3", "16"),
			},
			liveNodes: map[string]struct{}{"node-1": {}, "node-2": {}, "node-3": {}},
			expected:  28,
		},
		{
			name: "When NodeClaims have no registered node, it should skip them",
			nodeClaims: []karpenterv1.NodeClaim{
				nodeClaimWithCapacity("nc-1", "", "4"),
				nodeClaimWithCapacity("nc-2", "", "8"),
			},
			liveNodes: nil,
			expected:  0,
		},
		{
			name: "When NodeClaims have empty capacity, it should skip them",
			nodeClaims: []karpenterv1.NodeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "nc-1"},
					Status:     karpenterv1.NodeClaimStatus{NodeName: "node-1"},
				},
			},
			liveNodes: map[string]struct{}{"node-1": {}},
			expected:  0,
		},
		{
			name: "When there is a mix of registered and unregistered NodeClaims, it should only count registered ones",
			nodeClaims: []karpenterv1.NodeClaim{
				nodeClaimWithCapacity("nc-1", "node-1", "4"),
				nodeClaimWithCapacity("nc-2", "", "8"),
				nodeClaimWithCapacity("nc-3", "node-3", "16"),
				{
					ObjectMeta: metav1.ObjectMeta{Name: "nc-4"},
					Status:     karpenterv1.NodeClaimStatus{NodeName: "node-4"},
				},
			},
			liveNodes: map[string]struct{}{"node-1": {}, "node-3": {}, "node-4": {}},
			expected:  20,
		},
		{
			name: "When NodeClaim references a node that no longer exists, it should not count it",
			nodeClaims: []karpenterv1.NodeClaim{
				nodeClaimWithCapacity("nc-1", "node-1", "4"),
				nodeClaimWithCapacity("nc-2", "node-2", "8"),
			},
			liveNodes: map[string]struct{}{"node-1": {}},
			expected:  4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(sumNodeClaimVCPUs(tt.nodeClaims, tt.liveNodes)).To(Equal(tt.expected))
		})
	}
}

func nodeClaimWithCapacity(name, nodeName, cpus string) karpenterv1.NodeClaim {
	nc := karpenterv1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: karpenterv1.NodeClaimStatus{
			NodeName: nodeName,
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse(cpus),
			},
		},
	}
	return nc
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(corev1.AddToScheme(scheme)).To(Succeed())
	g.Expect(hyperv1beta1.AddToScheme(scheme)).To(Succeed())
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "karpenter.sh", Version: "v1", Kind: "NodePool",
	}, &karpenterv1.NodePool{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "karpenter.sh", Version: "v1", Kind: "NodePoolList",
	}, &karpenterv1.NodePoolList{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "karpenter.sh", Version: "v1", Kind: "NodeClaim",
	}, &karpenterv1.NodeClaim{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{
		Group: "karpenter.sh", Version: "v1", Kind: "NodeClaimList",
	}, &karpenterv1.NodeClaimList{})
	scheme.AddKnownTypeWithName(capiClusterGVK(), &metav1.PartialObjectMetadata{})
	return scheme
}

func capiClusterGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{Group: "cluster.x-k8s.io", Version: "v1beta2", Kind: "Cluster"}
}

func newCAPICluster(namespace, name string, deletionTimestamp time.Time) *metav1.PartialObjectMetadata {
	cluster := &metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			DeletionTimestamp: &metav1.Time{Time: deletionTimestamp},
			Finalizers:        []string{"capi-finalizer"},
		},
	}
	cluster.SetGroupVersionKind(capiClusterGVK())
	return cluster
}
