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

	SufficientNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "sufficient_nodes",
		Help:      "Whether Number of nodes under NodeTemplate is sufficient or not",
	}, []string{"nodetemplate"})
)

func init() {
	metrics.Registry.MustRegister(CurrentNodesVec)
	metrics.Registry.MustRegister(DesiredNodesVec)
	metrics.Registry.MustRegister(SufficientNodesVec)
}
