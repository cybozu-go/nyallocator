package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/config"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var _ = Describe("NodeTemplate Controller", func() {
	Context("When reconciling a resource", func() {
		var stopFunc func()
		ctx := context.Background()

		BeforeEach(func() {
			mgr, err := ctrl.NewManager(cfg, ctrl.Options{
				Scheme:         scheme,
				LeaderElection: false,
				Metrics: metricsserver.Options{
					BindAddress: ":8080",
				},
				Controller: config.Controller{
					SkipNameValidation: ptr.To(true),
				},
			})
			Expect(err).ToNot(HaveOccurred())
			nodeReconciler := &NodeReconciler{
				Client: mgr.GetClient(),
				Scheme: scheme,
			}
			Expect(nodeReconciler.SetupWithManager(mgr)).To(Succeed())
			ctx, cancel := context.WithCancel(ctx)
			stopFunc = cancel
			go func() {
				err := mgr.Start(ctx)
				if err != nil {
					panic(err)
				}
			}()
			time.Sleep(100 * time.Millisecond)
		})

		AfterEach(func() {
			err := k8sClient.DeleteAllOf(ctx, &corev1.Node{})
			Expect(err).ToNot(HaveOccurred())
			stopFunc()
			time.Sleep(100 * time.Millisecond)
		})

		It("should add spare label to nodes that have spare taint", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "test-label": "foo"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "test-label": "foo"}).build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}

			By("checking Node status")
			Eventually(func(g Gomega) {
				nodeList := &corev1.NodeList{}
				err := k8sClient.List(ctx, nodeList)
				g.Expect(err).ToNot(HaveOccurred())
				for _, node := range nodeList.Items {
					if node.Name == "node1" {
						g.Expect(node.Labels).To(HaveKeyWithValue("node-role.kubernetes.io/spare", "true"))
						g.Expect(node.Labels).To(HaveKeyWithValue("test-label", "foo"))
					} else if node.Name == "node2" {
						g.Expect(node.Labels).ToNot(HaveKey("node-role.kubernetes.io/spare"))
						g.Expect(node.Labels).To(HaveKeyWithValue("test-label", "foo"))
					}
				}
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})
	})
})
