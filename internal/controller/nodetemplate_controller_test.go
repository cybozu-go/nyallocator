package controller

import (
	"context"
	"io"
	"net/http"
	"time"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			nodeTemplateReconciler := &NodeTemplateReconciler{
				Client: mgr.GetClient(),
				Scheme: scheme,
			}
			Expect(nodeTemplateReconciler.SetupWithManager(mgr)).To(Succeed())
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
			err := k8sClient.DeleteAllOf(ctx, &nyallocatorv1.NodeTemplate{})
			Expect(err).ToNot(HaveOccurred())
			Eventually(func(g Gomega) {
				ntList := &nyallocatorv1.NodeTemplateList{}
				err := k8sClient.List(ctx, ntList)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(ntList.Items).To(BeEmpty(), "NodeTemplate should be deleted")
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
			err = k8sClient.DeleteAllOf(ctx, &corev1.Node{})
			Expect(err).ToNot(HaveOccurred())
			stopFunc()
			time.Sleep(100 * time.Millisecond)
		})

		It("should successfully allocate the nodes", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}
			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    2,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"test-label": "foo",
							},
							Annotations: map[string]string{
								"test-annotation": "bar",
							},
						},
						Spec: nyallocatorv1.NodeConfigurationSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test-taint",
									Value:  "baz",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("checking node is properly configured")
			n := &corev1.NodeList{}
			err = k8sClient.List(ctx, n, client.MatchingLabels{"nyallocator.cybozu.io/node-template": "test"})
			Expect(err).ToNot(HaveOccurred())
			Expect(n.Items).To(HaveLen(2), "should have allocated 2 nodes")
			for _, node := range n.Items {
				Expect(node.Labels).To(HaveKeyWithValue("test-label", "foo"), "node should have the correct label")
				Expect(node.Annotations).To(HaveKeyWithValue("test-annotation", "bar"), "node should have the correct annotation")
				Expect(node.Spec.Taints).To(ContainElement(corev1.Taint{
					Key:    "test-taint",
					Value:  "baz",
					Effect: corev1.TaintEffectNoSchedule,
				}), "node should have the correct taint")
			}

			By("checking metrics are exposed correctly")
			resp, err := http.Get("http://localhost:8080/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test\"} 1"))
		})

		It("should not allocate nodes when dry run is enabled", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}
			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   true,
					Priority: 1,
					Nodes:    2,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", false)

			By("checking Node status")
			Eventually(func(g Gomega) {
				nodes := &corev1.NodeList{}
				err := k8sClient.List(ctx, nodes)
				g.Expect(err).ToNot(HaveOccurred())
				allUnchanged := true
			L:
				for _, node := range nodes.Items {
					for _, taint := range node.Spec.Taints {
						if taint.Key == "node.cybozu.io/spare" {
							continue L
						}
					}
					allUnchanged = false
				}
				g.Expect(allUnchanged).To(BeTrue())
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should allocate nodes by following the order of priority", func() {
			By("creating NodeTemplate")
			// Create NodeTemplates before creating Nodes to allocate the controller can handle multiple templates correctly.
			nodeTemplate1 := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test1",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 10,
					Nodes:    2,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			}
			nodeTemplate2 := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test2",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 100,
					Nodes:    2,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate1)
			Expect(err).ToNot(HaveOccurred())
			err = k8sClient.Create(ctx, nodeTemplate2)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test1", false)
			checkNodeTemplateStatus("test2", false)

			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test1", false)
			checkNodeTemplateStatus("test2", true)

			By("checking nodes are created according to priority")
			Eventually(func(g Gomega) {
				var n1, n2 corev1.NodeList
				err := k8sClient.List(ctx, &n1, client.MatchingLabels{"nyallocator.cybozu.io/node-template": "test1"})
				g.Expect(err).ToNot(HaveOccurred())
				err = k8sClient.List(ctx, &n2, client.MatchingLabels{"nyallocator.cybozu.io/node-template": "test2"})
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(n1.Items).To(HaveLen(1))
				g.Expect(n2.Items).To(HaveLen(2))
			}).WithTimeout(20 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("checking metrics are exposed correctly")
			resp, err := http.Get("http://localhost:8080/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test1\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test2\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test1\"} 1"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test2\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test1\"} 0"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test2\"} 1"))

			By("adding a new spare node")
			newNode := newNode("node4").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build()
			err = k8sClient.Create(ctx, &newNode)
			Expect(err).ToNot(HaveOccurred())

			By("checking node4 is used")
			Eventually(func(g Gomega) {
				node := &corev1.Node{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "node4"}, node)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(node.Labels).To(HaveKeyWithValue("nyallocator.cybozu.io/node-template", "test1"))
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test1", true)
			checkNodeTemplateStatus("test2", true)

			By("checking metrics are exposed correctly")
			resp, err = http.Get("http://localhost:8080/metrics")
			Expect(err).ToNot(HaveOccurred())
			body, err = io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test1\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test2\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test1\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test2\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test1\"} 1"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test2\"} 1"))
		})

		It("should replace nodes when node is deleted", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}
			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    3,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"foo": "bar",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("deleting an existing node")
			err = k8sClient.Delete(ctx, &nodes[0])
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status after deletion")
			checkNodeTemplateStatus("test", false)

			By("checking metrics are exposed correctly")
			resp, err := http.Get("http://localhost:8080/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test\"} 3"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test\"} 2"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test\"} 0"))

			By("adding a new spare node")
			newNode := newNode("node4").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build()
			err = k8sClient.Create(ctx, &newNode)
			Expect(err).ToNot(HaveOccurred())

			By("checking node4 is used")
			Eventually(func(g Gomega) {
				node := &corev1.Node{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "node4"}, node)
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(node.Labels).To(HaveKeyWithValue("nyallocator.cybozu.io/node-template", "test"))
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("checking metrics are exposed correctly")
			resp, err = http.Get("http://localhost:8080/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			body, err = io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test\"} 3"))
			Expect(string(body)).To(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test\"} 3"))
			Expect(string(body)).To(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test\"} 1"))
		})

		It("should update nodes when the NodeTemplate is updated", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}
			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    3,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"test-label": "foo",
							},
							Annotations: map[string]string{
								"test-annotation": "foo",
							},
						},
						Spec: nyallocatorv1.NodeConfigurationSpec{
							Taints: []corev1.Taint{
								{
									Key:    "test-taint",
									Value:  "foo",
									Effect: corev1.TaintEffectNoSchedule,
								},
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("checking Node status")
			allNodes := &corev1.NodeList{}
			err = k8sClient.List(ctx, allNodes)
			Expect(err).ToNot(HaveOccurred())

			for _, node := range allNodes.Items {
				Expect(node.Labels).To(HaveKeyWithValue("test-label", "foo"), "node should have the correct label")
			}

			By("updating NodeTemplate")
			nodeTemplate = &nyallocatorv1.NodeTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test"}, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())
			nodeTemplate.Spec.Template.Metadata.Labels["test-label"] = "bar"
			nodeTemplate.Spec.Template.Metadata.Labels["new-label"] = "baz"
			nodeTemplate.Spec.Template.Metadata.Annotations["test-annotation"] = "bar"
			nodeTemplate.Spec.Template.Metadata.Annotations["new-annotation"] = "baz"
			nodeTemplate.Spec.Template.Spec.Taints[0] = corev1.Taint{
				Key:    "test-taint",
				Value:  "bar",
				Effect: corev1.TaintEffectNoExecute,
			}
			nodeTemplate.Spec.Template.Spec.Taints = append(nodeTemplate.Spec.Template.Spec.Taints, corev1.Taint{
				Key:    "new-taint",
				Value:  "baz",
				Effect: corev1.TaintEffectNoSchedule,
			})

			err = k8sClient.Update(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status after update")
			checkNodeTemplateStatus("test", true)

			By("checking Node status after update")
			allNodes = &corev1.NodeList{}
			err = k8sClient.List(ctx, allNodes)
			Expect(err).ToNot(HaveOccurred())
			for _, node := range allNodes.Items {
				Expect(node.Labels).To(HaveKeyWithValue("test-label", "bar"))
				Expect(node.Labels).To(HaveKeyWithValue("new-label", "baz"))
				Expect(node.Annotations).To(HaveKeyWithValue("test-annotation", "bar"))
				Expect(node.Annotations).To(HaveKeyWithValue("new-annotation", "baz"))
				Expect(node.Spec.Taints).To(ContainElement(corev1.Taint{
					Key:    "test-taint",
					Value:  "bar",
					Effect: corev1.TaintEffectNoExecute,
				}))
				Expect(node.Spec.Taints).To(ContainElement(corev1.Taint{
					Key:    "new-taint",
					Value:  "baz",
					Effect: corev1.TaintEffectNoSchedule,
				}), "node should have the new taint node:"+node.Name)
			}

			By("removing label, annotation, taint from NodeTemplate")
			nodeTemplate = &nyallocatorv1.NodeTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test"}, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())
			delete(nodeTemplate.Spec.Template.Metadata.Labels, "new-label")
			delete(nodeTemplate.Spec.Template.Metadata.Annotations, "new-annotation")
			nodeTemplate.Spec.Template.Spec.Taints = []corev1.Taint{
				{
					Key:    "test-taint",
					Value:  "bar",
					Effect: corev1.TaintEffectNoExecute,
				},
			}
			err = k8sClient.Update(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status after update")
			checkNodeTemplateStatus("test", true)

			By("checking label, annotation, taint are not removed from Node")
			allNodes = &corev1.NodeList{}
			err = k8sClient.List(ctx, allNodes)
			Expect(err).ToNot(HaveOccurred())
			for _, node := range allNodes.Items {
				Expect(node.Labels).To(HaveKeyWithValue("test-label", "bar"))
				Expect(node.Labels).To(HaveKeyWithValue("new-label", "baz"))
				Expect(node.Annotations).To(HaveKeyWithValue("test-annotation", "bar"))
				Expect(node.Annotations).To(HaveKeyWithValue("new-annotation", "baz"))
				Expect(node.Spec.Taints).To(ContainElement(corev1.Taint{
					Key:    "test-taint",
					Value:  "bar",
					Effect: corev1.TaintEffectNoExecute,
				}))
				Expect(node.Spec.Taints).To(ContainElement(corev1.Taint{
					Key:    "new-taint",
					Value:  "baz",
					Effect: corev1.TaintEffectNoSchedule,
				}), "node should have the new taint node:"+node.Name)
			}
		})

		It("should sync configurations of nodes when node is updated", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}
			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    3,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"test-label": "foo",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("changing a node's configuration")
			nodeToUpdate := &corev1.Node{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "node1"}, nodeToUpdate)
			Expect(err).ToNot(HaveOccurred())
			nodeToUpdate.Labels["test-label"] = "updated"
			err = k8sClient.Update(ctx, nodeToUpdate)
			Expect(err).ToNot(HaveOccurred())

			By("checking node's configuration is synced")
			Eventually(func(g Gomega) {
				updatedNode := &corev1.Node{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "node1"}, updatedNode)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(updatedNode.Labels).To(HaveKeyWithValue("test-label", "foo"))
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
		})

		It("should select node based on score", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack0"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack1"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack2"}).withSpareTaint().build(),
				newNode("node4").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack0"}).withSpareTaint().build(),
				newNode("node5").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack1"}).withSpareTaint().build(),
				newNode("node6").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack2"}).withSpareTaint().build(),
				newNode("node7").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack0"}).withSpareTaint().build(),
				newNode("node8").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack1"}).withSpareTaint().build(),
				newNode("node9").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true", "topology.kubernetes.io/zone": "rack2"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}
			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    6,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"test-label": "foo",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("counting nodes in each zone")
			allocatedNode := &corev1.NodeList{}
			err = k8sClient.List(ctx, allocatedNode, client.MatchingLabels{"nyallocator.cybozu.io/node-template": "test"})
			Expect(err).ToNot(HaveOccurred())
			zoneCount := make(map[string]int)
			for _, node := range allocatedNode.Items {
				if val, ok := node.Labels["topology.kubernetes.io/zone"]; ok {
					zoneCount[val]++
				}
			}
			for key, count := range zoneCount {
				Expect(count).To(BeNumerically("==", 2), "each zone should have 2 nodes, but got %d in zone %s", count, key)
			}
		})

		It("should select nodes based on matching labels", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/type1": "true", "topology.kubernetes.io/zone": "rack0", "custom-label": "foo"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/type1": "true", "topology.kubernetes.io/zone": "rack1", "custom-label": "foo"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/type1": "true", "topology.kubernetes.io/zone": "rack2", "custom-label": "foo"}).withSpareTaint().build(),
				newNode("node4").withLabel(map[string]string{"node-role.kubernetes.io/type2": "true", "topology.kubernetes.io/zone": "rack0", "custom-label": "bar"}).withSpareTaint().build(),
				newNode("node5").withLabel(map[string]string{"node-role.kubernetes.io/type2": "true", "topology.kubernetes.io/zone": "rack1", "custom-label": "bar"}).withSpareTaint().build(),
				newNode("node6").withLabel(map[string]string{"node-role.kubernetes.io/type2": "true", "topology.kubernetes.io/zone": "rack2", "custom-label": "bar"}).withSpareTaint().build(),
				newNode("node7").withLabel(map[string]string{"node-role.kubernetes.io/type2": "true", "topology.kubernetes.io/zone": "rack0", "custom-label": "foo"}).withSpareTaint().build(),
				newNode("node8").withLabel(map[string]string{"node-role.kubernetes.io/type2": "true", "topology.kubernetes.io/zone": "rack1", "custom-label": "foo"}).withSpareTaint().build(),
				newNode("node9").withLabel(map[string]string{"node-role.kubernetes.io/type2": "true", "topology.kubernetes.io/zone": "rack2", "custom-label": "foo"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}

			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    1,
					Selector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "custom-label",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"foo"},
							},
							{
								Key:      "node-role.kubernetes.io/type2",
								Operator: metav1.LabelSelectorOpIn,
								Values:   []string{"true"},
							},
							{
								Key:      "topology.kubernetes.io/zone",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{"rack0", "rack1"},
							},
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"test-label": "foo",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("checking Node status")
			allocatedNode := &corev1.NodeList{}
			err = k8sClient.List(ctx, allocatedNode, client.MatchingLabels{"nyallocator.cybozu.io/node-template": "test"})
			Expect(err).ToNot(HaveOccurred())
			Expect(allocatedNode.Items).To(HaveLen(1), "should have created 1 node")
			Expect(allocatedNode.Items[0].Name).To(Equal("node9"), "should have selected node9 based on the label selector")
		})

		It("should remove reference label from nodes when NodeTemplate is deleted", func() {
			By("creating Node")
			nodes := []corev1.Node{
				newNode("node1").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node2").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
				newNode("node3").withLabel(map[string]string{"node-role.kubernetes.io/worker": "true"}).withSpareTaint().build(),
			}
			for _, node := range nodes {
				err := k8sClient.Create(ctx, &node)
				Expect(err).ToNot(HaveOccurred())
			}

			By("creating NodeTemplate")
			nodeTemplate := &nyallocatorv1.NodeTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
				Spec: nyallocatorv1.NodeTemplateSpec{
					DryRun:   false,
					Priority: 1,
					Nodes:    3,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"node-role.kubernetes.io/worker": "true",
						},
					},
					Template: nyallocatorv1.NodeConfiguration{
						Metadata: nyallocatorv1.NodeConfigurationMetadata{
							Labels: map[string]string{
								"test-label": "foo",
							},
						},
					},
				},
			}
			err := k8sClient.Create(ctx, nodeTemplate)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate status")
			checkNodeTemplateStatus("test", true)

			By("checking NodeTemplate has a finalizer")
			nt := &nyallocatorv1.NodeTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: "test"}, nt)
			Expect(err).ToNot(HaveOccurred())
			Expect(nt.ObjectMeta.Finalizers).To(ContainElement("nyallocator.cybozu.io/finalizer"))

			By("deleting NodeTemplate")
			err = k8sClient.Delete(ctx, nt)
			Expect(err).ToNot(HaveOccurred())

			By("checking NodeTemplate is deleted")
			Eventually(func(g Gomega) {
				nt := &nyallocatorv1.NodeTemplate{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "test"}, nt)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("checking nodes has no reference label")
			allNodes := &corev1.NodeList{}
			err = k8sClient.List(ctx, allNodes)
			Expect(err).ToNot(HaveOccurred())
			for _, node := range allNodes.Items {
				Expect(node.Labels).ToNot(HaveKey("nyallocator.cybozu.io/node-template"))
			}

			By("checking metrics are exposed correctly")
			resp, err := http.Get("http://localhost:8080/metrics")
			Expect(err).ToNot(HaveOccurred())
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			Expect(err).ToNot(HaveOccurred())
			Expect(string(body)).NotTo(ContainSubstring("nyallocator_desired_nodes{nodetemplate=\"test\"}"))
			Expect(string(body)).NotTo(ContainSubstring("nyallocator_current_nodes{nodetemplate=\"test\"}"))
			Expect(string(body)).NotTo(ContainSubstring("nyallocator_sufficient_nodes{nodetemplate=\"test\"}"))
		})
	})
})

type NodeBuilder struct {
	corev1.Node
}

func newNode(name string) NodeBuilder {
	return NodeBuilder{
		Node: corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

func (n NodeBuilder) withSpareTaint() NodeBuilder {
	n.Spec.Taints = append(n.Spec.Taints, corev1.Taint{
		Key:    "node.cybozu.io/spare",
		Value:  "true",
		Effect: corev1.TaintEffectNoSchedule,
	})
	return n
}

func (n NodeBuilder) withLabel(labels map[string]string) NodeBuilder {
	if n.ObjectMeta.Labels == nil {
		n.ObjectMeta.Labels = make(map[string]string)
	}
	for k, v := range labels {
		n.ObjectMeta.Labels[k] = v
	}
	return n
}
func (n NodeBuilder) build() corev1.Node {
	return n.Node
}

func checkNodeTemplateStatus(name string, expectSufficient bool) {
	Eventually(func(g Gomega) {
		nt := &nyallocatorv1.NodeTemplate{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name}, nt)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(nt.Status.ReconcileInfo.Generation).To(Equal(nt.ObjectMeta.Generation))
		if expectSufficient {
			g.Expect(nt.Status.Sufficient).To(BeTrue())
		} else {
			g.Expect(nt.Status.Sufficient).To(BeFalse())
		}
	}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
}
