package controller

import (
	"context"
	"reflect"
	"slices"
	"time"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	SpareTaintKey                 = "node.cybozu.io/spare"
	SpareRoleLabelKey             = "node-role.kubernetes.io/spare"
	NodeTemplateReferenceLabelKey = "nyallocator.cybozu.io/node-template"
	ZoneLabelKey                  = "topology.kubernetes.io/zone"
	NodeTemplateFinalizer         = "nyallocator.cybozu.io/finalizer"
)

type NodeTemplateReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	nodeTemplate := &nyallocatorv1.NodeTemplate{}
	err = r.Client.Get(ctx, client.ObjectKey{Name: req.Name}, nodeTemplate)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// add finalizer if not exists
	if nodeTemplate.DeletionTimestamp == nil && nodeTemplate.Finalizers == nil {
		nodeTemplate.Finalizers = []string{NodeTemplateFinalizer}
		err = r.Update(ctx, nodeTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	existingNodes := corev1.NodeList{}
	err = r.Client.List(ctx, &existingNodes, client.MatchingLabels{NodeTemplateReferenceLabelKey: nodeTemplate.Name})
	if err != nil {
		return ctrl.Result{}, err
	}

	if nodeTemplate.DeletionTimestamp != nil {
		for _, node := range existingNodes.Items {
			// remove the reference label
			delete(node.Labels, NodeTemplateReferenceLabelKey)
			log.Info("node template is being deleted, removing reference label",
				"node", node.Name,
				"NodeTemplate", nodeTemplate.Name,
			)
			err = r.Client.Update(ctx, &node)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		// remove finalizer
		nodeTemplate.Finalizers = nil
		log.Info("removing finalizer", "NodeTemplate", nodeTemplate.Name, "finalizer", nodeTemplate.Finalizers)
		err = r.Update(ctx, nodeTemplate)
		if err != nil {
			return ctrl.Result{}, err
		}
		// remove metrics
		r.removeMetrics(nodeTemplate)
		return ctrl.Result{}, nil
	}

	defer func() {
		updateStatusErr := r.updateStatus(ctx, nodeTemplate)
		if updateStatusErr != nil {
			err = updateStatusErr
		}
		// if nodeTemplate.Status.Sufficient is not true, requeue after 1 second
		if !nodeTemplate.Status.Sufficient {
			result = ctrl.Result{
				RequeueAfter: time.Second * 1,
			}
		}
	}()

	// sync existing nodes
	for _, node := range existingNodes.Items {
		nodeOriginal := node.DeepCopy()
		r.reflectNodeConfiguration(&node, nodeTemplate)
		if !reflect.DeepEqual(*nodeOriginal, node) {
			log.Info("updating node",
				"node", node.Name,
				"NodeTemplate", nodeTemplate.Name,
			)
			if nodeTemplate.Spec.DryRun {
				log.Info("dry run mode, skipping update nodes.\n diff:\n"+calcDiff(nodeOriginal, &node),
					"node", node.Name,
					"NodeTemplate", nodeTemplate.Name,
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

	// if there are NodeTemplates that is lack of nodes with higher priority, skip ensuring of nodes.
	nodeTemplates := nyallocatorv1.NodeTemplateList{}
	err = r.Client.List(ctx, &nodeTemplates)
	if err != nil {
		return ctrl.Result{}, err
	}
	for _, nt := range nodeTemplates.Items {
		if nt.Spec.Priority > nodeTemplate.Spec.Priority && !nt.Status.Sufficient {
			log.Info("skip ensuring node, because there is a NodeTemplate with higher priority that is not sufficient",
				"target", nodeTemplate.Name,
				"prior", nt.Name,
				"current", nt.Status.CurrentNodes,
				"expected", nt.Spec.Nodes,
			)
			return ctrl.Result{}, nil
		}
	}

	// allocate new nodes
	allNode := corev1.NodeList{}
	spareNodes := map[string]corev1.Node{}
	err = r.Client.List(ctx, &allNode)
	if err != nil {
		return ctrl.Result{}, err
	}

	for _, node := range allNode.Items {
		for _, taint := range node.Spec.Taints {
			if taint.Key == SpareTaintKey {
				selector, err := metav1.LabelSelectorAsSelector(nodeTemplate.Spec.Selector)
				if err != nil {
					return ctrl.Result{}, err
				}
				if selector.Matches(labels.Set(node.Labels)) {
					spareNodes[node.Name] = node
				}
			}
		}
	}

	countNodeByZone := make(map[string]int)
	for _, n := range existingNodes.Items {
		if zone, ok := n.Labels[ZoneLabelKey]; ok {
			countNodeByZone[zone]++
		}
	}

	for i := len(existingNodes.Items); i < nodeTemplate.Spec.Nodes; i++ {
		if len(spareNodes) == 0 {
			log.Info("no spare nodes found", "target", nodeTemplate.Name)
			continue
		}

		candidate := selectNodes(spareNodes, countNodeByZone)
		delete(spareNodes, candidate.Name)
		if zone, ok := candidate.Labels[ZoneLabelKey]; ok {
			countNodeByZone[zone]++
		}
		candidateOriginal := candidate.DeepCopy()

		// remove spare taint
		for i, taint := range candidate.Spec.Taints {
			if taint.Key == SpareTaintKey {
				candidate.Spec.Taints = slices.Delete(candidate.Spec.Taints, i, i+1)
			}
		}

		// remove spare label
		delete(candidate.Labels, SpareRoleLabelKey)

		// add reference label
		candidate.Labels[NodeTemplateReferenceLabelKey] = nodeTemplate.Name

		r.reflectNodeConfiguration(&candidate, nodeTemplate)
		if nodeTemplate.Spec.DryRun {
			log.Info("dry run mode, skipping update nodes.\n diff:\n"+calcDiff(candidateOriginal, &candidate),
				"node", candidate.Name,
				"NodeTemplate", nodeTemplate.Name,
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
			"NodeTemplate", nodeTemplate.Name,
		)
	}

	return ctrl.Result{}, nil
}

func (r *NodeTemplateReconciler) reflectNodeConfiguration(node *corev1.Node, nodeTemplate *nyallocatorv1.NodeTemplate) {
	if nodeTemplate.Spec.Template.Spec.Taints != nil && node.Spec.Taints == nil {
		node.Spec.Taints = []corev1.Taint{}
	}
L:
	for _, taint := range nodeTemplate.Spec.Template.Spec.Taints {
		for i, t := range node.Spec.Taints {
			if t.Key == taint.Key {
				node.Spec.Taints[i] = taint
				break L
			}
		}
		node.Spec.Taints = append(node.Spec.Taints, taint)
	}
	if nodeTemplate.Spec.Template.Metadata.Labels != nil && node.Labels == nil {
		node.Labels = make(map[string]string)
	}
	node.Labels["node-role.kubernetes.io"+"/"+nodeTemplate.Name] = "true"
	for k, v := range nodeTemplate.Spec.Template.Metadata.Labels {
		node.Labels[k] = v
	}
	if nodeTemplate.Spec.Template.Metadata.Annotations != nil && node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}
	for k, v := range nodeTemplate.Spec.Template.Metadata.Annotations {
		node.Annotations[k] = v
	}
}

func selectNodes(spareNodes map[string]corev1.Node, zoneCount map[string]int) corev1.Node {
	maxCountPerZone := 100
	maxScore := -1
	maxScoreNode := ""
	for k, n := range spareNodes {
		zoneCountScore := 0
		if zone, ok := n.Labels[ZoneLabelKey]; ok {
			zoneCountScore = maxCountPerZone - zoneCount[zone]
		}
		score := zoneCountScore * 10
		if score > maxScore {
			maxScore = score
			maxScoreNode = k
		}
	}
	return spareNodes[maxScoreNode]
}

func (r *NodeTemplateReconciler) updateStatus(ctx context.Context, nodeTemplate *nyallocatorv1.NodeTemplate) error {
	nodes := corev1.NodeList{}
	err := r.Client.List(ctx, &nodes, client.MatchingLabels{NodeTemplateReferenceLabelKey: nodeTemplate.Name})
	if err != nil {
		return err
	}
	nodeTemplate.Status.ReconcileInfo.Generation = nodeTemplate.Generation
	nodeTemplate.Status.CurrentNodes = len(nodes.Items)
	nodeTemplate.Status.Sufficient = nodeTemplate.Status.CurrentNodes >= nodeTemplate.Spec.Nodes
	r.updateMetrics(nodeTemplate)
	err = r.Client.Status().Update(ctx, nodeTemplate)
	if err != nil {
		return err
	}
	return nil
}

func (r *NodeTemplateReconciler) updateMetrics(nodeTemplate *nyallocatorv1.NodeTemplate) {
	if nodeTemplate.Status.Sufficient {
		SufficientNodesVec.WithLabelValues(nodeTemplate.Name).Set(1)
	} else {
		SufficientNodesVec.WithLabelValues(nodeTemplate.Name).Set(0)
	}
	CurrentNodesVec.WithLabelValues(nodeTemplate.Name).Set(float64(nodeTemplate.Status.CurrentNodes))
	ExpectedNodesVec.WithLabelValues(nodeTemplate.Name).Set(float64(nodeTemplate.Spec.Nodes))
}

func (r *NodeTemplateReconciler) removeMetrics(nodeTemplate *nyallocatorv1.NodeTemplate) {
	SufficientNodesVec.DeleteLabelValues(nodeTemplate.Name)
	CurrentNodesVec.DeleteLabelValues(nodeTemplate.Name)
	ExpectedNodesVec.DeleteLabelValues(nodeTemplate.Name)
}

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := corev1.Node{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name}, &node)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return ctrl.Result{}, err
		}
	}
	for _, taint := range node.Spec.Taints {
		if taint.Key == SpareTaintKey {
			if _, ok := node.Labels[SpareRoleLabelKey]; !ok {
				// if spare node but does not have spare role label, add it
				node.Labels[SpareRoleLabelKey] = "true"
				err := r.Client.Update(ctx, &node)
				if err != nil {
					return ctrl.Result{}, err
				}
			}
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nyallocatorv1.NodeTemplate{}).
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var requests []reconcile.Request
				nodeLabels := obj.GetLabels()
				if val, ok := nodeLabels[NodeTemplateReferenceLabelKey]; ok {
					nodeTemplate := &nyallocatorv1.NodeTemplate{}
					err := r.Client.Get(ctx, client.ObjectKey{Name: val}, nodeTemplate)
					if err != nil {
						logf.Log.Info("failed to get NodeTemplate", "name", val)
						return []reconcile.Request{}
					}
					requests = append(requests, reconcile.Request{
						NamespacedName: client.ObjectKey{
							Name: nodeTemplate.Name,
						},
					})
				}
				return requests
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
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func calcDiff(old, new *corev1.Node) string {
	diff := cmp.Diff(old, new, cmpopts.IgnoreFields(*old, "Status"))
	return diff
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Named("node").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
