package controller

import (
	"context"
	"math"
	"reflect"
	"slices"
	"time"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	SpareTaintKey                    = "node.cybozu.io/spare"
	SpareRoleLabelKey                = "node-role.kubernetes.io/spare"
	NodeTemplateReferenceLabelKey    = "nyallocator.cybozu.io/node-template"
	OutOfManagementNodesLabelKey     = "nyallocator.cybozu.io/out-of-management"
	OutOfManagementNodesRoleLabelKey = "node-role.kubernetes.io/out-of-management"
	ZoneLabelKey                     = "topology.kubernetes.io/zone"
	NodeTemplateFinalizer            = "nyallocator.cybozu.io/finalizer"
	ConditionSufficient              = "Sufficient"
	ConditionReconcileSuccess        = "ReconcileSuccess"
)

type InsufficientReason struct {
	HighPriorityNodeTemplateExists bool
	NoSpareNodesFound              bool
}

type NodeTemplateReconciler struct {
	client.Client
	Scheme             *runtime.Scheme
	RequeueRateLimiter workqueue.TypedRateLimiter[ctrl.Request]
}
type NodeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nyallocator.cybozu.io,resources=nodetemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nyallocator.cybozu.io,resources=nodetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nyallocator.cybozu.io,resources=nodetemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch

func (r *NodeTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	log := logf.FromContext(ctx)
	statusReason := InsufficientReason{}

	nodeTemplate := &nyallocatorv1.NodeTemplate{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: req.Name}, nodeTemplate)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// add finalizer if not exists
	if nodeTemplate.DeletionTimestamp == nil && controllerutil.ContainsFinalizer(nodeTemplate, NodeTemplateFinalizer) == false {
		controllerutil.AddFinalizer(nodeTemplate, NodeTemplateFinalizer)
		err = r.Update(ctx, nodeTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{
			RequeueAfter: time.Second,
		}, nil
	}

	hasReferenceLabelNodes := corev1.NodeList{}
	err = r.Client.List(ctx, &hasReferenceLabelNodes, client.MatchingLabels{NodeTemplateReferenceLabelKey: nodeTemplate.Name})
	if err != nil {
		return ctrl.Result{}, err
	}

	if nodeTemplate.DeletionTimestamp != nil {
		for _, node := range hasReferenceLabelNodes.Items {
			// remove the reference label
			delete(node.Labels, NodeTemplateReferenceLabelKey)
			log.Info("node template is being deleted, removing reference label",
				"node", node.Name,
				"nodetemplate", nodeTemplate.Name,
			)
			err = r.Client.Update(ctx, &node)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		// remove metrics
		r.removeMetrics(nodeTemplate)

		// remove finalizer
		updated := controllerutil.RemoveFinalizer(nodeTemplate, NodeTemplateFinalizer)
		if updated {
			log.Info("removing finalizer", "nodetemplate", nodeTemplate.Name, "finalizer", nodeTemplate.Finalizers)
			err = r.Update(ctx, nodeTemplate)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	defer func(reason *InsufficientReason) {
		updateStatusErr := r.updateStatus(ctx, nodeTemplate, reason, err)
		if updateStatusErr != nil {
			err = updateStatusErr
			return
		}
		// if nodeTemplate.Status.Sufficient is not true, exponential backoff by enabling requeue.
		if !isSufficient(nodeTemplate) {
			after := r.RequeueRateLimiter.When(req)
			result = ctrl.Result{
				RequeueAfter: after,
			}
		} else {
			r.RequeueRateLimiter.Forget(req)
		}
	}(&statusReason)

	selector, err := metav1.LabelSelectorAsSelector(nodeTemplate.Spec.Selector)
	if err != nil {
		return ctrl.Result{}, err
	}

	underManagementNodes := []corev1.Node{}
	outOfManagementNodes := []corev1.Node{}
	for _, node := range hasReferenceLabelNodes.Items {
		if selector.Matches(labels.Set(node.Labels)) {
			underManagementNodes = append(underManagementNodes, node)
		} else {
			outOfManagementNodes = append(outOfManagementNodes, node)
		}
	}
	if len(outOfManagementNodes) > 0 {
		for _, node := range outOfManagementNodes {
			node.Labels[OutOfManagementNodesRoleLabelKey] = "true"
			node.Labels[OutOfManagementNodesLabelKey] = nodeTemplate.Name
			delete(node.Labels, NodeTemplateReferenceLabelKey)
			err := r.Client.Update(ctx, &node)
			if err != nil {
				return ctrl.Result{}, err
			}
			log.Info("node does not match the selector,removing reference label and adding out-of-management labels",
				"node", node.Name,
				"nodetemplate", nodeTemplate.Name,
			)
		}
		return ctrl.Result{}, nil
	}

	// sync existing nodes
	for _, node := range underManagementNodes {
		nodeOriginal := node.DeepCopy()
		r.reflectNodeConfiguration(&node, nodeTemplate)
		if !reflect.DeepEqual(*nodeOriginal, node) {
			log.Info("updating node",
				"node", node.Name,
				"nodetemplate", nodeTemplate.Name,
			)
			if nodeTemplate.Spec.DryRun {
				log.Info("dry run mode, skipping update nodes.\n diff:\n"+calcDiff(nodeOriginal, &node),
					"node", node.Name,
					"nodetemplate", nodeTemplate.Name,
					"phase", "updating",
				)
				continue
			}
			err = r.Client.Update(ctx, &node)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	if isSufficient(nodeTemplate) {
		return ctrl.Result{}, err
	}

	// if there are NodeTemplates that is lack of nodes with higher priority, skip ensuring of nodes.
	nodeTemplates := nyallocatorv1.NodeTemplateList{}
	err = r.Client.List(ctx, &nodeTemplates)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, nt := range nodeTemplates.Items {
		if nt.Spec.Priority > nodeTemplate.Spec.Priority && !isSufficient(&nt) {
			log.Info("skip ensuring node, because there is a NodeTemplate with higher priority that is not sufficient",
				"nodetemplate", nodeTemplate.Name,
				"prior", nt.Name,
			)
			statusReason.HighPriorityNodeTemplateExists = true
			return ctrl.Result{}, nil
		}
	}

	// allocate new nodes
	allNodes := corev1.NodeList{}
	spareNodes := map[string]corev1.Node{}
	err = r.Client.List(ctx, &allNodes)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, node := range allNodes.Items {
		if _, ok := node.Labels[NodeTemplateReferenceLabelKey]; !ok && node.Labels[SpareRoleLabelKey] == "true" {
			if selector.Matches(labels.Set(node.Labels)) {
				spareNodes[node.Name] = node
			}
		}
	}

	countNodeByZone := make(map[string]int)
	for _, n := range underManagementNodes {
		if zone, ok := n.Labels[ZoneLabelKey]; ok {
			countNodeByZone[zone]++
		}
	}

	for i := len(underManagementNodes); i < nodeTemplate.Spec.Nodes; i++ {
		if len(spareNodes) == 0 {
			log.Info("no spare nodes found", "nodetemplate", nodeTemplate.Name)
			statusReason.NoSpareNodesFound = true
			return ctrl.Result{}, nil
		}

		candidate := selectNodes(spareNodes, countNodeByZone)
		delete(spareNodes, candidate.Name)
		if zone, ok := candidate.Labels[ZoneLabelKey]; ok {
			countNodeByZone[zone]++
		}
		candidateOriginal := candidate.DeepCopy()

		r.reflectNodeConfiguration(&candidate, nodeTemplate)
		if nodeTemplate.Spec.DryRun {
			log.Info("dry run mode, skipping update nodes.\n diff:\n"+calcDiff(candidateOriginal, &candidate),
				"node", candidate.Name,
				"nodetemplate", nodeTemplate.Name,
				"phase", "ensuring",
			)
			continue
		}
		err := r.Client.Update(ctx, &candidate)
		if err != nil {
			return ctrl.Result{}, err
		}
		log.Info("allocated node",
			"node", candidate.Name,
			"nodetemplate", nodeTemplate.Name,
		)
	}

	return ctrl.Result{}, nil
}

func (r *NodeTemplateReconciler) reflectNodeConfiguration(node *corev1.Node, nodeTemplate *nyallocatorv1.NodeTemplate) {
	if node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	// remove spare taint and label, and add node template reference label
	for i, taint := range node.Spec.Taints {
		if taint.Key == SpareTaintKey {
			node.Spec.Taints = slices.Delete(node.Spec.Taints, i, i+1)
		}
	}
	delete(node.Labels, SpareRoleLabelKey)
	node.Labels[NodeTemplateReferenceLabelKey] = nodeTemplate.Name

L:
	for _, taint := range nodeTemplate.Spec.Template.Spec.Taints {
		for i, t := range node.Spec.Taints {
			if t.Key == taint.Key {
				node.Spec.Taints[i] = taint
				continue L
			}
		}
		node.Spec.Taints = append(node.Spec.Taints, taint)
	}

	node.Labels["node-role.kubernetes.io"+"/"+nodeTemplate.Name] = "true"
	for k, v := range nodeTemplate.Spec.Template.Metadata.Labels {
		node.Labels[k] = v
	}

	for k, v := range nodeTemplate.Spec.Template.Metadata.Annotations {
		node.Annotations[k] = v
	}
}

func selectNodes(spareNodes map[string]corev1.Node, zoneCount map[string]int) corev1.Node {
	maxCountPerZone := math.MaxInt
	maxScore := -1
	maxScoreNode := ""
	for k, n := range spareNodes {
		zoneCountScore := 0
		if zone, ok := n.Labels[ZoneLabelKey]; ok {
			zoneCountScore = maxCountPerZone - zoneCount[zone]
		}
		score := zoneCountScore
		if score > maxScore {
			maxScore = score
			maxScoreNode = k
		}
	}
	return spareNodes[maxScoreNode]
}

func (r *NodeTemplateReconciler) updateStatus(ctx context.Context, nodeTemplate *nyallocatorv1.NodeTemplate, reason *InsufficientReason, reconcileError error) error {
	nodes := corev1.NodeList{}
	err := r.Client.List(ctx, &nodes, client.MatchingLabels{NodeTemplateReferenceLabelKey: nodeTemplate.Name})
	if err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsSelector(nodeTemplate.Spec.Selector)
	if err != nil {
		return err
	}
	underManagementNodes := []corev1.Node{}
	for _, node := range nodes.Items {
		if selector.Matches(labels.Set(node.Labels)) {
			underManagementNodes = append(underManagementNodes, node)
		}
	}

	nodeTemplate.Status.ReconcileInfo.ObservedGeneration = nodeTemplate.Generation

	nodeTemplate.Status.CurrentNodes = len(underManagementNodes)

	conditionSufficient := metav1.Condition{
		Type: ConditionSufficient,
	}

	if nodeTemplate.Status.CurrentNodes < nodeTemplate.Spec.Nodes {
		conditionSufficient.Status = metav1.ConditionFalse
		if reason.HighPriorityNodeTemplateExists {
			conditionSufficient.Reason = "HighPriorityNodeTemplateExists"
			conditionSufficient.Message = "There is a NodeTemplate with higher priority that is not sufficient"
		} else if reason.NoSpareNodesFound {
			conditionSufficient.Reason = "NoSpareNodesFound"
			conditionSufficient.Message = "No spare nodes found"
		} else {
			conditionSufficient.Reason = "ReconcileError"
			conditionSufficient.Message = "Current nodes are insufficient because of reconcile error"
		}
	} else {
		conditionSufficient.Status = metav1.ConditionTrue
		conditionSufficient.Reason = "SufficientNodes"
		conditionSufficient.Message = "Current nodes are sufficient"
	}
	conditionReconcileSuccess := metav1.Condition{
		Type: ConditionReconcileSuccess,
	}
	if reconcileError != nil {
		conditionReconcileSuccess.Status = metav1.ConditionFalse
		conditionReconcileSuccess.Reason = "ReconcileError"
		conditionReconcileSuccess.Message = reconcileError.Error()
	} else {
		conditionReconcileSuccess.Status = metav1.ConditionTrue
		conditionReconcileSuccess.Reason = "ReconcileSuccess"
		conditionReconcileSuccess.Message = "Reconcile succeeded"
	}

	meta.SetStatusCondition(&nodeTemplate.Status.Conditions, conditionSufficient)
	meta.SetStatusCondition(&nodeTemplate.Status.Conditions, conditionReconcileSuccess)

	r.updateMetrics(nodeTemplate)
	err = r.Client.Status().Update(ctx, nodeTemplate)
	if err != nil {
		return err
	}
	return nil
}

func (r *NodeTemplateReconciler) updateMetrics(nodeTemplate *nyallocatorv1.NodeTemplate) {
	if isSufficient(nodeTemplate) {
		SufficientNodesVec.WithLabelValues(nodeTemplate.Name).Set(1)
	} else {
		SufficientNodesVec.WithLabelValues(nodeTemplate.Name).Set(0)
	}
	CurrentNodesVec.WithLabelValues(nodeTemplate.Name).Set(float64(nodeTemplate.Status.CurrentNodes))
	DesiredNodesVec.WithLabelValues(nodeTemplate.Name).Set(float64(nodeTemplate.Spec.Nodes))
	for _, condition := range nodeTemplate.Status.Conditions {
		if condition.Type == ConditionReconcileSuccess && condition.Status == metav1.ConditionTrue {
			ReconcileSuccessVec.WithLabelValues(nodeTemplate.Name).Set(1)
		} else if condition.Type == ConditionReconcileSuccess && condition.Status == metav1.ConditionFalse {
			ReconcileSuccessVec.WithLabelValues(nodeTemplate.Name).Set(0)
		}
	}
}

func (r *NodeTemplateReconciler) removeMetrics(nodeTemplate *nyallocatorv1.NodeTemplate) {
	SufficientNodesVec.DeleteLabelValues(nodeTemplate.Name)
	CurrentNodesVec.DeleteLabelValues(nodeTemplate.Name)
	DesiredNodesVec.DeleteLabelValues(nodeTemplate.Name)
	SpareNodesVec.DeleteLabelValues(nodeTemplate.Name)
	ReconcileSuccessVec.DeleteLabelValues(nodeTemplate.Name)
}

func isSufficient(nodeTemplate *nyallocatorv1.NodeTemplate) bool {
	for _, condition := range nodeTemplate.Status.Conditions {
		if condition.Type == ConditionSufficient && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nyallocatorv1.NodeTemplate{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				nodeLabels := obj.GetLabels()
				if val, ok := nodeLabels[NodeTemplateReferenceLabelKey]; ok {
					return []reconcile.Request{
						{
							NamespacedName: client.ObjectKey{
								Name: val,
							},
						},
					}
				}
				return nil
			}),
			builder.WithPredicates(
				predicate.And(
					predicate.Or(
						predicate.GenerationChangedPredicate{}, // when Spec changes
						predicate.LabelChangedPredicate{},
						predicate.AnnotationChangedPredicate{},
					),
					predicate.NewTypedPredicateFuncs(func(object client.Object) bool {
						if _, ok := object.GetLabels()[NodeTemplateReferenceLabelKey]; ok {
							return true
						}
						return false
					}),
				),
			),
		).
		Named("nodetemplate").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}). // This must be 1 to avoid conflicts between NodeTemplates.
		Complete(r)
}

func calcDiff(old, new *corev1.Node) string {
	diff := cmp.Diff(old, new, cmpopts.IgnoreFields(*old, "Status"))
	return diff
}
