package controller

import (
	"context"

	corev1 "k8s.io/api/core/v1"
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
	return ctrl.Result{}, nil
}

func (r *NodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Node{}).
		Named("node").
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}
