package controller

import (
	"context"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// +kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch

func (r *NodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := corev1.Node{}
	err := r.Client.Get(ctx, client.ObjectKey{Name: req.Name}, &node)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for _, taint := range node.Spec.Taints {
		if taint.Key == SpareTaintKey {
			if node.Labels[SpareRoleLabelKey] != "true" {
				// if spare node but does not have spare role label, add it
				if node.Labels == nil {
					node.Labels = make(map[string]string)
				}
				node.Labels[SpareRoleLabelKey] = "true"
				err := r.Client.Update(ctx, &node)
				if err != nil {
					return ctrl.Result{}, client.IgnoreNotFound(err)
				}
			}
		}
	}
	err = r.updateMetrics(ctx)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *NodeReconciler) updateMetrics(ctx context.Context) error {
	nodeTemplates := &nyallocatorv1.NodeTemplateList{}
	err := r.Client.List(ctx, nodeTemplates)
	if err != nil {
		return err
	}
	for _, nodeTemplate := range nodeTemplates.Items {
		selector, err := metav1.LabelSelectorAsSelector(nodeTemplate.Spec.Selector)
		if err != nil {
			return err
		}
		allNodes := corev1.NodeList{}
		err = r.Client.List(ctx, &allNodes)
		if err != nil {
			return err
		}
		spareNodes := []corev1.Node{}
		for _, node := range allNodes.Items {
			if _, ok := node.Labels[NodeTemplateReferenceLabelKey]; !ok && node.Labels[SpareRoleLabelKey] == "true" {
				if selector.Matches(labels.Set(node.Labels)) {
					spareNodes = append(spareNodes, node)
				}
			}
		}
		SpareNodesVec.WithLabelValues(nodeTemplate.Name).Set(float64(len(spareNodes)))
	}
	return nil
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Named("node").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
