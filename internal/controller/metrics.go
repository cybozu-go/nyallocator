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
		Name:      "current",
		Help:      "Number of nodes under the NodeTemplate",
	}, []string{"name"})
	ExpectedNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "expected",
		Help:      "Number of nodes specified in the NodeTemplate",
	}, []string{"name"})

	SufficientNodesVec = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "sufficient",
		Help:      "Whether Number of nodes under NodeTemplate is sufficient or not",
	}, []string{"name"})
)

func init() {
	metrics.Registry.MustRegister(CurrentNodesVec)
	metrics.Registry.MustRegister(ExpectedNodesVec)
	metrics.Registry.MustRegister(SufficientNodesVec)
}
