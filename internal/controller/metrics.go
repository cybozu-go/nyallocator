package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	metricsNamespace = "nyallocator"
)

var (
	CurrentNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "current_nodes",
		Help:      "Number of nodes under the NodeTemplate",
	}, []string{"nodetemplate"})
	DesiredNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "desired_nodes",
		Help:      "Number of nodes specified in the NodeTemplate",
	}, []string{"nodetemplate"})
	SpareNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "spare_nodes",
		Help:      "Number of spare nodes matching the NodeTemplate",
	}, []string{"nodetemplate"})
	SufficientNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "sufficient_nodes",
		Help:      "Whether number of nodes under the NodeTemplate is sufficient or not",
	}, []string{"nodetemplate"})
	ReconcileSuccessVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "reconcile_success",
		Help:      "Whether reconciliations of the NodeTemplate is successful or not",
	}, []string{"nodetemplate"})
)

func init() {
	metrics.Registry.MustRegister(CurrentNodesVec)
	metrics.Registry.MustRegister(DesiredNodesVec)
	metrics.Registry.MustRegister(SufficientNodesVec)
	metrics.Registry.MustRegister(SpareNodesVec)
	metrics.Registry.MustRegister(ReconcileSuccessVec)
}
