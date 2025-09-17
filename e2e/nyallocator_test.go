package e2e

import (
	"encoding/json"
	"io"
	"net/http"
	"os/exec"
	"time"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("nyallocator e2e test", func() {
	It("should set spare label", func() {
		By("getting nodes")
		nodes := &corev1.NodeList{}
		out, stderr, err := kubectl(nil, "get", "nodes", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes.Items).To(HaveLen(9))
		for _, n := range nodes.Items {
			for _, taint := range n.Spec.Taints {
				if taint.Key == "nyallocator.cybozu.io/spare" {
					Expect(n.Labels).To(HaveKeyWithValue("node-role.kubernetes.io/spare", "true"))

				}
			}
		}
	})

	It("should allocate nodes", func() {
		By("applying NodeTemplate manifest")
		_, stderr, err := kubectl(nil, "apply", "-f", "./manifests/nodetemplate.yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)

		By("checking node templates")
		Eventually(func(g Gomega) {
			nt := nyallocatorv1.NodeTemplateList{}
			out, stderr, err := kubectl(nil, "get", "nodetemplates", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred(), stderr)
			err = yaml.Unmarshal(out, &nt)
			g.Expect(err).NotTo(HaveOccurred())
			for _, n := range nt.Items {
				g.Expect(isSufficient(&n)).To(BeTrue())
			}
		}).Should(Succeed(), "Node templates should be sufficient")

		By("checking nodes")
		cp := &corev1.NodeList{}
		cs := &corev1.NodeList{}
		ss := &corev1.NodeList{}

		out, stderr, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=control-plane", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, cp)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Items).To(HaveLen(3))
		for _, n := range cp.Items {
			Expect(n.Labels).To(HaveKeyWithValue("node-role.kubernetes.io/control-plane", "true"))
		}
		out, stderr, err = kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, cs)
		Expect(err).NotTo(HaveOccurred())
		Expect(cs.Items).To(HaveLen(2))
		for _, n := range cs.Items {
			Expect(n.Labels).To(HaveKeyWithValue("cke.cybozu.com/role", "cs"))
		}

		out, stderr, err = kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=ss", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, ss)
		Expect(err).NotTo(HaveOccurred())
		Expect(ss.Items).To(HaveLen(2))
		for _, n := range ss.Items {
			Expect(n.Labels).To(HaveKeyWithValue("cke.cybozu.com/role", "ss"))
		}

		By("checking the metrics")
		portForwardCmd := exec.Command("./bin/kubectl", "port-forward", "-n", "nyallocator-system", "deployments/nyallocator-controller-manager", "8080:8080")
		portForwardCmd.Start()
		defer portForwardCmd.Process.Kill()

		Eventually(func(g Gomega) {
			res, err := http.Get("http://localhost:8080/metrics")
			g.Expect(err).NotTo(HaveOccurred())
			defer res.Body.Close()
			g.Expect(res.StatusCode).To(Equal(http.StatusOK))
			metrics, err := io.ReadAll(res.Body)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_current_nodes{nodetemplate="control-plane"} 3`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_desired_nodes{nodetemplate="control-plane"} 3`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_sufficient_nodes{nodetemplate="control-plane"} 1`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_spare_nodes{nodetemplate="control-plane"} 0`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_reconcile_success{nodetemplate="control-plane"} 1`))

			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_current_nodes{nodetemplate="cs"} 2`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_desired_nodes{nodetemplate="cs"} 2`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_sufficient_nodes{nodetemplate="cs"} 1`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_spare_nodes{nodetemplate="cs"} 1`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_reconcile_success{nodetemplate="cs"} 1`))

			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_current_nodes{nodetemplate="ss"} 2`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_desired_nodes{nodetemplate="ss"} 2`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_sufficient_nodes{nodetemplate="ss"} 1`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_spare_nodes{nodetemplate="ss"} 1`))
			g.Expect(string(metrics)).To(ContainSubstring(`nyallocator_reconcile_success{nodetemplate="ss"} 1`))

		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})

	It("should allocate node when templates are updated", func() {
		By("updating NodeTemplate manifest")
		nt := &nyallocatorv1.NodeTemplate{}
		out, stderr, err := kubectl(nil, "get", "nodetemplates", "ss", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, nt)
		Expect(err).NotTo(HaveOccurred())
		generation := nt.ObjectMeta.Generation

		_, stderr, err = kubectl(nil, "patch", "nodetemplates", "ss", "--type=merge", "-p", `{"spec":{"nodes":3}}`)
		Expect(err).NotTo(HaveOccurred(), stderr)

		By("checking node templates after update")
		Eventually(func(g Gomega) {
			ntList := nyallocatorv1.NodeTemplateList{}
			out, stderr, err := kubectl(nil, "get", "nodetemplates", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred(), stderr)
			err = yaml.Unmarshal(out, &ntList)
			g.Expect(err).NotTo(HaveOccurred())
			for _, n := range ntList.Items {
				if n.Name == "ss" {
					g.Expect(n.Status.ReconcileInfo.ObservedGeneration).To(Equal(generation + 1))
				}
				g.Expect(isSufficient(&n)).To(BeTrue())
			}
		}).Should(Succeed(), "Node templates should be sufficient after update")
	})

	It("should allocate node when node is deleted", func() {
		By("deleting a node")
		nodes := &corev1.NodeList{}
		out, stderr, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes.Items).To(HaveLen(2))
		nodeName := nodes.Items[0].Name
		_, stderr, err = kubectl(nil, "delete", "node", nodeName)
		Expect(err).NotTo(HaveOccurred(), stderr)

		By("checking nodes")
		Eventually(func(g Gomega) {
			cs := &corev1.NodeList{}
			out, stderr, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred(), stderr)
			err = yaml.Unmarshal(out, cs)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(cs.Items).To(HaveLen(2))
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})

	It("should change status if insufficient", func() {
		By("updating NodeTemplate manifest")
		nt := &nyallocatorv1.NodeTemplate{}
		out, stderr, err := kubectl(nil, "get", "nodetemplates", "ss", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, nt)
		Expect(err).NotTo(HaveOccurred())
		generation := nt.ObjectMeta.Generation

		_, stderr, err = kubectl(nil, "patch", "nodetemplates", "ss", "--type=merge", "-p", `{"spec":{"nodes":4}}`)
		Expect(err).NotTo(HaveOccurred(), stderr)

		By("checking node templates after update")
		Eventually(func(g Gomega) {
			nt := &nyallocatorv1.NodeTemplate{}
			out, stderr, err := kubectl(nil, "get", "nodetemplates", "ss", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred(), stderr)
			err = yaml.Unmarshal(out, nt)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(nt.Status.ReconcileInfo.ObservedGeneration).To(Equal(generation + 1))
			for _, condition := range nt.Status.Conditions {
				if condition.Type == "Sufficient" {
					g.Expect(condition.Status).To(Equal(metav1.ConditionFalse))
					g.Expect(condition.Reason).To(Equal("NoSpareNodesFound"))
				}
			}
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})

	It("should success delete NodeTemplate", func() {
		By("deleting a NodeTemplate")
		_, stderr, err := kubectl(nil, "delete", "nodetemplates", "ss")
		Expect(err).NotTo(HaveOccurred(), stderr)

		By("checking nodes")
		Eventually(func(g Gomega) {
			ss := &corev1.NodeList{}
			out, stderr, err := kubectl(nil, "get", "nodes", "-l", "cke.cybozu.com/role=ss", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred(), stderr)
			err = yaml.Unmarshal(out, ss)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(ss.Items).To(HaveLen(3))
			for _, n := range ss.Items {
				g.Expect(n.Labels).NotTo(HaveKey("nyallocator.cybozu.io/node-template"))
			}
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		By("checking NodeTemplate")
		_, stderr, err = kubectl(nil, "get", "nodetemplates", "ss")
		Expect(err).To(HaveOccurred(), stderr)
		Expect(string(stderr)).To(ContainSubstring("not found"))

	})

	It("should allow edit of non-reference labels", func() {
		By("getting a node without reference label")
		cs := &corev1.NodeList{}
		out, stderr, err := kubectl(nil, "get", "nodes", "-l", `!nyallocator.cybozu.io/node-template`, "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), string(stderr))
		err = yaml.Unmarshal(out, cs)
		Expect(err).NotTo(HaveOccurred())
		Expect(cs.Items).NotTo(BeEmpty())
		Expect(cs.Items[0].Labels).NotTo(HaveKey("nyallocator.cybozu.io/node-template"))

		By("editing a node label")
		_, _, err = kubectl(nil, "label", "node", cs.Items[0].Name, "foo=bar")
		Expect(err).NotTo(HaveOccurred())
	})

	It("should not allow edit of reference labels", func() {
		By("getting a node")
		cs := &corev1.NodeList{}
		out, stderr, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred(), stderr)
		err = yaml.Unmarshal(out, cs)
		Expect(err).NotTo(HaveOccurred())
		nodeName := cs.Items[0].Name

		By("editing a node label")
		_, stderr, err = kubectl(nil, "label", "node", nodeName, "nyallocator.cybozu.io/node-template=foo", "--overwrite")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))

		_, stderr, err = kubectl(nil, "label", "node", nodeName, "nyallocator.cybozu.io/node-template-", "--overwrite")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))
	})

	It("should not allow use nyallocator reserved key in nodetemplate", func() {
		By("creating a NodeTemplate with spare label")
		nt := &nyallocatorv1.NodeTemplate{
			TypeMeta:   metav1.TypeMeta{Kind: "NodeTemplate", APIVersion: nyallocatorv1.GroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "test1"},
			Spec: nyallocatorv1.NodeTemplateSpec{
				Nodes: 1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
				Template: nyallocatorv1.NodeConfiguration{
					Metadata: nyallocatorv1.NodeConfigurationMetadata{
						Labels: map[string]string{
							"node-role.kubernetes.io/spare": "true",
						},
					},
				},
			},
		}
		jsonData, err := json.Marshal(nt)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err := kubectl(jsonData, "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))

		By("creating a NodeTemplate with reference label")
		nt = &nyallocatorv1.NodeTemplate{
			TypeMeta:   metav1.TypeMeta{Kind: "NodeTemplate", APIVersion: nyallocatorv1.GroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "test2"},
			Spec: nyallocatorv1.NodeTemplateSpec{
				Nodes: 1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
				Template: nyallocatorv1.NodeConfiguration{
					Metadata: nyallocatorv1.NodeConfigurationMetadata{
						Labels: map[string]string{
							"nyallocator.cybozu.io/node-template": "test2",
						},
					},
				},
			},
		}
		jsonData, err = json.Marshal(nt)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err = kubectl(jsonData, "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))

		By("creating a NodeTemplate with spare taint")
		nt = &nyallocatorv1.NodeTemplate{
			TypeMeta:   metav1.TypeMeta{Kind: "NodeTemplate", APIVersion: nyallocatorv1.GroupVersion.String()},
			ObjectMeta: metav1.ObjectMeta{Name: "test3"},
			Spec: nyallocatorv1.NodeTemplateSpec{
				Nodes: 1,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"test": "test",
					},
				},
				Template: nyallocatorv1.NodeConfiguration{
					Spec: nyallocatorv1.NodeConfigurationSpec{
						Taints: []corev1.Taint{
							{
								Key:    "nyallocator.cybozu.io/spare",
								Value:  "true",
								Effect: corev1.TaintEffectNoSchedule,
							},
						},
					},
				},
			},
		}
		jsonData, err = json.Marshal(nt)
		Expect(err).NotTo(HaveOccurred())
		_, stderr, err = kubectl(jsonData, "apply", "-f", "-")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))
	})
})
