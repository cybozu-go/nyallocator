package controller

import (
	"reflect"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

func sortTaints(ts []corev1.Taint) []corev1.Taint {
	out := make([]corev1.Taint, len(ts))
	copy(out, ts)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key != out[j].Key {
			return out[i].Key < out[j].Key
		}
		if out[i].Value != out[j].Value {
			return out[i].Value < out[j].Value
		}
		return out[i].Effect < out[j].Effect
	})
	return out
}

func taintsEqual(a, b []corev1.Taint) bool {
	return reflect.DeepEqual(sortTaints(a), sortTaints(b))
}

func taintChangePredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool { return true },
		DeleteFunc: func(e event.DeleteEvent) bool { return true },
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldNode, okOld := e.ObjectOld.(*corev1.Node)
			newNode, okNew := e.ObjectNew.(*corev1.Node)
			if !okOld || !okNew {
				return false
			}
			return !taintsEqual(oldNode.Spec.Taints, newNode.Spec.Taints)
		},
		GenericFunc: func(e event.GenericEvent) bool { return false },
	}
}
