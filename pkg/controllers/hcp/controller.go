package hcp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openshift/karpenter-operator/pkg/hypershift"

	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

const (
	// karpenterFinalizer is the finalizer added to HostedControlPlane resources by the karpenter-operator.
	// This allows time for Karpenter to delete all NodePools and NodeClaims before the HostedControlPlane is deleted.
	// Its value is shared with the HyperShift operator, which removes it as a fallback, so it must not change.
	karpenterFinalizer = "hypershift.openshift.io/karpenter-finalizer"

	// NodeClaimDeletionTimeout is the timeout for the deletion of a NodeClaim during cluster deletion.
	// If the timeout is reached, the NodeClaim will be forcefully deleted by setting the termination timestamp annotation.
	NodeClaimDeletionTimeout = 3 * time.Minute

	// KarpenterDeletionRequeueInterval is the interval at which the controller will requeue deletion of Karpenter resources during a hosted cluster deletion.
	KarpenterDeletionRequeueInterval = 15 * time.Second
)

// ControllerConfig configures HostedControlPlane lifecycle reconciliation.
type ControllerConfig struct {
	HostedCluster cluster.Cluster
	Namespace     string
}

// Controller reconciles Karpenter resources in the hosted cluster during HostedControlPlane lifecycle events.
type Controller struct {
	config           *ControllerConfig
	hostedCluster    cluster.Cluster
	hostedClient     client.Client
	managementClient client.Client
}

func NewController(mgr ctrl.Manager, cfg *ControllerConfig) *Controller {
	c := &Controller{
		config:           cfg,
		managementClient: mgr.GetClient(),
	}
	if cfg.HostedCluster != nil {
		c.hostedClient = cfg.HostedCluster.GetClient()
		c.hostedCluster = cfg.HostedCluster
	}
	return c
}

func (c *Controller) Name() string {
	return "hostedcontrolplane"
}

func (c *Controller) SetupWithManager(mgr ctrl.Manager) error { //nolint:gocyclo
	if c.hostedCluster == nil {
		return errors.New("hosted cluster is required")
	}

	ctrlr, err := controller.New(c.Name(), mgr, controller.Options{Reconciler: c})
	if err != nil {
		return fmt.Errorf("constructing controller: %w", err)
	}

	namespacedPredicates := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetNamespace() == c.config.Namespace
	})
	// Watch the HCP management side.
	if err := ctrlr.Watch(source.Kind[client.Object](mgr.GetCache(), &hyperv1beta1.HostedControlPlane{}, handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, o client.Object) []ctrl.Request {
			if o.GetNamespace() != c.config.Namespace {
				return nil
			}
			return []ctrl.Request{{NamespacedName: client.ObjectKeyFromObject(o)}}
		},
	), namespacedPredicates)); err != nil {
		return fmt.Errorf("watching HostedControlPlane: %w", err)
	}

	// Watch the CAPI Cluster management side. The CAPI Cluster is deleted earlier
	// than the HCP in the HostedCluster deletion sequence, so reacting to it lets
	// us start Karpenter node cleanup in parallel with regular CAPI node teardown
	// rather than waiting for the HCP DeletionTimestamp (which is set much later).
	capiCluster := &metav1.PartialObjectMetadata{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta2",
		Kind:    "Cluster",
	})
	if err := ctrlr.Watch(source.Kind[client.Object](mgr.GetCache(), capiCluster, handler.EnqueueRequestsFromMapFunc(
		func(ctx context.Context, o client.Object) []ctrl.Request {
			if o.GetNamespace() != c.config.Namespace {
				return nil
			}
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: c.config.Namespace}}}
		},
	), namespacedPredicates)); err != nil {
		return fmt.Errorf("watching CAPI Cluster: %w", err)
	}

	// Only enqueue on Add/Delete — not status-only updates — since reconcileAutoNodeStatus cares only
	// about count changes (nodes joining or leaving the cluster).
	countChangePredicate := predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return true },
		DeleteFunc:  func(e event.DeleteEvent) bool { return true },
		UpdateFunc:  func(e event.UpdateEvent) bool { return false },
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}

	// NodeClaim predicate: fire on create/delete (count changes) and also when
	// CPU capacity changes (for vCPU billing). Capacity is populated after the
	// node registers, so we need updates for that transition.
	nodeClaimPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return true },
		DeleteFunc: func(e event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNC, ok1 := e.ObjectOld.(*karpenterv1.NodeClaim)
			newNC, ok2 := e.ObjectNew.(*karpenterv1.NodeClaim)
			if !ok1 || !ok2 {
				return false
			}
			oldCPU := oldNC.Status.Capacity[corev1.ResourceCPU]
			newCPU := newNC.Status.Capacity[corev1.ResourceCPU]
			if !oldCPU.Equal(newCPU) {
				return true
			}
			return oldNC.Status.NodeName != newNC.Status.NodeName
		},
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}

	// Watch NodeClaims hosted side to trigger reconcile when NodeClaims change.
	if err := ctrlr.Watch(source.Kind[client.Object](c.hostedCluster.GetCache(), &karpenterv1.NodeClaim{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []ctrl.Request {
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: c.config.Namespace}}}
		}),
		nodeClaimPredicate,
	)); err != nil {
		return fmt.Errorf("watching NodeClaims: %w", err)
	}

	// Watch Nodes hosted side to trigger reconcile when node counts change.
	if err := ctrlr.Watch(source.Kind[client.Object](c.hostedCluster.GetCache(), &corev1.Node{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, _ client.Object) []ctrl.Request {
			return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: c.config.Namespace}}}
		}),
		countChangePredicate,
	)); err != nil {
		return fmt.Errorf("watching Nodes: %w", err)
	}

	// Trigger initial sync.
	initialSync := make(chan event.GenericEvent)
	if err := ctrlr.Watch(source.Channel(initialSync, &handler.EnqueueRequestForObject{})); err != nil {
		return fmt.Errorf("watching initial sync channel: %w", err)
	}
	go func() {
		initialSync <- event.GenericEvent{Object: &hyperv1beta1.HostedControlPlane{}}
	}()

	return nil
}

func (c *Controller) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	hcp, err := hypershift.GetHostedControlPlane(ctx, c.managementClient, c.config.Namespace)
	if err != nil {
		if errors.Is(err, hypershift.ErrHostedControlPlaneNotFound) {
			log.V(1).Info("HostedControlPlane not found, requeueing")
			return ctrl.Result{RequeueAfter: time.Second * 5}, nil
		}
		return ctrl.Result{}, err
	}

	clusterDeleting, err := c.isClusterDeleting(ctx, hcp)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("checking cluster deletion state: %w", err)
	}
	if clusterDeleting {
		return c.reconcileDeletion(ctx, hcp)
	}
	if !controllerutil.ContainsFinalizer(hcp, karpenterFinalizer) {
		originalHCP := hcp.DeepCopy()
		controllerutil.AddFinalizer(hcp, karpenterFinalizer)
		if err := c.managementClient.Patch(ctx, hcp, client.MergeFromWithOptions(originalHCP, client.MergeFromWithOptimisticLock{})); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer to HostedControlPlane: %w", err)
		}
	}

	if err := c.reconcileAutoNodeStatus(ctx, hcp); err != nil {
		return ctrl.Result{}, fmt.Errorf("reconciling AutoNode status: %w", err)
	}

	return ctrl.Result{}, nil
}

// reconcileAutoNodeStatus counts Karpenter-managed nodes and live NodeClaims in the hosted cluster
// and writes the counts to HCP.Status.AutoNode.
func (c *Controller) reconcileAutoNodeStatus(ctx context.Context, hcp *hyperv1beta1.HostedControlPlane) error {
	nodes := &corev1.NodeList{}
	if err := c.hostedClient.List(ctx, nodes); err != nil {
		return fmt.Errorf("listing nodes: %w", err)
	}

	liveNodes := make(map[string]struct{}, len(nodes.Items))
	var karpenterNodeCount int32
	for i := range nodes.Items {
		liveNodes[nodes.Items[i].Name] = struct{}{}
		if _, hasLabel := nodes.Items[i].Labels[karpenterv1.NodePoolLabelKey]; hasLabel {
			karpenterNodeCount++
		}
	}

	// TODO(jkyros): this includes nodeclaims where the nodeclaim is
	// being deleted, we can filter on deletion timestamp if we don't want those in the list
	nodeClaims := &karpenterv1.NodeClaimList{}
	if err := c.hostedClient.List(ctx, nodeClaims); err != nil {
		return fmt.Errorf("listing NodeClaims: %w", err)
	}

	vcpus := sumNodeClaimVCPUs(nodeClaims.Items, liveNodes)

	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(hyperv1beta1.GroupVersion.WithKind("HostedControlPlane"))
	u.SetName(hcp.Name)
	u.SetNamespace(hcp.Namespace)
	u.Object["status"] = map[string]any{
		"autoNode": map[string]any{
			"nodeCount":      int64(karpenterNodeCount),
			"nodeClaimCount": int64(len(nodeClaims.Items)),
			"vcpus":          int64(vcpus),
		},
	}
	if err := c.managementClient.Status().Apply(ctx, client.ApplyConfigurationFromUnstructured(u),
		client.FieldOwner("karpenter-operator"), client.ForceOwnership); err != nil {
		return fmt.Errorf("applying AutoNode status: %w", err)
	}
	return nil
}

// sumNodeClaimVCPUs returns the total vCPU count across NodeClaims whose
// backing Node still exists in the cluster and has reported CPU capacity.
// The NodeClaim is the authoritative record of Karpenter ownership.
func sumNodeClaimVCPUs(nodeClaims []karpenterv1.NodeClaim, liveNodes map[string]struct{}) int32 {
	var total int64
	for i := range nodeClaims {
		nc := &nodeClaims[i]
		if _, ok := liveNodes[nc.Status.NodeName]; !ok {
			continue
		}
		if cpu, ok := nc.Status.Capacity[corev1.ResourceCPU]; ok {
			total += cpu.Value()
		}
	}
	return int32(total)
}

func (c *Controller) reconcileDeletion(ctx context.Context, hcp *hyperv1beta1.HostedControlPlane) (ctrl.Result, error) { //nolint:gocyclo
	log := ctrl.LoggerFrom(ctx)

	// TODO(maxcao13): if supporting disablement, we don't want to force delete immediately.
	// When force=true, we skip the graceful timeout and immediately trigger forceful deletion.
	// When force=false, we wait for NodeClaimDeletionTimeout before triggering forceful deletion.
	force := true
	if controllerutil.ContainsFinalizer(hcp, karpenterFinalizer) {
		// The deletion flow is:
		// 1. Delete all NodePools (NodeClaims will be marked for deletion from deleting the NodePools due to ownerReferences)
		// 2. Make sure all NodeClaims are actually gone (gracefully first, unless force=true)
		// 3. If graceful timeout or force=true, set the termination timestamp annotation to trigger Karpenter's forceful deletion
		// 4. Remove the finalizer from the HostedControlPlane to allow the rest of the HCP deletion to complete

		// Karpenter itself will make sure Nodes objects are deleted (and underlying instances are terminated) before finalizing the NodeClaims
		nodePoolList := &karpenterv1.NodePoolList{}
		if err := c.hostedClient.List(ctx, nodePoolList); err != nil {
			return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, fmt.Errorf("listing NodePools: %w", err)
		}

		// Delete all NodePools first
		if len(nodePoolList.Items) > 0 {
			for _, nodePool := range nodePoolList.Items {
				if !nodePool.GetDeletionTimestamp().IsZero() {
					continue
				}
				if err := c.hostedClient.Delete(ctx, &nodePool, &client.DeleteOptions{
					GracePeriodSeconds: new(int64(0)),
				}); err != nil {
					return ctrl.Result{}, fmt.Errorf("deleting NodePool: %w", err)
				}
			}
			return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, nil
		}

		// Make sure all NodeClaims are actually gone (gracefully first)
		nodeClaimList := &karpenterv1.NodeClaimList{}
		if err := c.hostedClient.List(ctx, nodeClaimList); err != nil {
			return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, fmt.Errorf("listing NodeClaims: %w", err)
		}
		if len(nodeClaimList.Items) > 0 {
			var elapsed time.Duration
			for _, nodeClaim := range nodeClaimList.Items {
				if nodeClaim.DeletionTimestamp == nil {
					log.Info("NodeClaim has no deletion timestamp during deletion; deleting explicitly", "nodeclaim", &nodeClaim)
					if err := c.hostedClient.Delete(ctx, &nodeClaim, &client.DeleteOptions{GracePeriodSeconds: new(int64(0))}); err != nil {
						return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, fmt.Errorf("deleting NodeClaim: %w", err)
					}
					continue
				}
				elapsed = time.Since(nodeClaim.DeletionTimestamp.Time)
				if !force && elapsed < NodeClaimDeletionTimeout {
					continue
				}

				if err := c.handleForcefulNodeClaimDeletion(ctx, &nodeClaim); err != nil {
					return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, fmt.Errorf("handling forceful NodeClaim deletion: %w", err)
				}
			}
			log.V(1).Info("Waiting for NodeClaims to be deleted", "nodeclaim count", len(nodeClaimList.Items), "elapsed", elapsed)
			return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, nil
		}
	}

	// Only remove the finalizer once the HCP itself is being deleted.
	// When triggered by the CAPI Cluster deletion alone, we clean up nodes
	// but leave the finalizer in place — it will be removed on a subsequent
	// reconcile when the HCP gets its own DeletionTimestamp.
	if hcp.DeletionTimestamp != nil {
		originalHCP := hcp.DeepCopy()
		controllerutil.RemoveFinalizer(hcp, karpenterFinalizer)
		if err := c.managementClient.Patch(ctx, hcp, client.MergeFromWithOptions(originalHCP, client.MergeFromWithOptimisticLock{})); err != nil {
			return ctrl.Result{RequeueAfter: KarpenterDeletionRequeueInterval}, fmt.Errorf("removing finalizer from HostedControlPlane: %w", err)
		}
		log.Info("Removed Karpenter finalizer from HostedControlPlane", "hostedcontrolplane", hcp)
	}
	return ctrl.Result{}, nil
}

// isClusterDeleting returns true when the cluster is being torn down.
// It checks both the HCP DeletionTimestamp and the CAPI Cluster DeletionTimestamp.
// The CAPI Cluster is deleted earlier in the HostedCluster deletion sequence than
// the HCP, so checking it allows node cleanup to begin sooner — in parallel with
// regular CAPI node teardown instead of after it completes.
func (c *Controller) isClusterDeleting(ctx context.Context, hcp *hyperv1beta1.HostedControlPlane) (bool, error) {
	if hcp.DeletionTimestamp != nil {
		return true, nil
	}

	if hcp.Spec.InfraID == "" {
		return false, nil
	}

	capiCluster := &metav1.PartialObjectMetadata{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta2",
		Kind:    "Cluster",
	})
	if err := c.managementClient.Get(ctx, client.ObjectKey{
		Namespace: c.config.Namespace,
		Name:      hcp.Spec.InfraID,
	}, capiCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("getting CAPI Cluster: %w", err)
	}

	return !capiCluster.DeletionTimestamp.IsZero(), nil
}

// handleForcefulNodeClaimDeletion handles the timeout of a NodeClaim during cluster deletion.
func (c *Controller) handleForcefulNodeClaimDeletion(ctx context.Context, nodeClaim *karpenterv1.NodeClaim) error {
	log := ctrl.LoggerFrom(ctx)

	// Check if we've already attempted termination
	if nodeClaim.Annotations[karpenterv1.NodeClaimTerminationTimestampAnnotationKey] != "" {
		log.V(1).Info("NodeClaim termination already attempted, skipping", "nodeclaim", nodeClaim)
		return nil
	}

	log.Info("Allowing Karpenter to forcefully delete NodeClaim", "nodeclaim", nodeClaim)

	// TODO(maxcao13): upstream has a escape hatch to forcefully delete NodeClaims using this annotation
	// there is an upstream issue to enable forceful deletion through a better interface: https://github.com/kubernetes-sigs/karpenter/issues/2815
	// we should come back later to fix this when that is resolved: https://issues.redhat.com/browse/AUTOSCALE-527
	patch := client.MergeFrom(nodeClaim.DeepCopy())
	if nodeClaim.Annotations == nil {
		nodeClaim.Annotations = make(map[string]string)
	}
	nodeClaim.Annotations[karpenterv1.NodeClaimTerminationTimestampAnnotationKey] = nodeClaim.GetDeletionTimestamp().Format(time.RFC3339)
	if err := c.hostedClient.Patch(ctx, nodeClaim, patch); err != nil {
		return fmt.Errorf("applying NodeClaim termination annotation: %w", err)
	}

	return nil
}
