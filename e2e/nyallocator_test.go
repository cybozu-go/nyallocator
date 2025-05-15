package e2e

import (
	"time"

	nyallocatorv1 "github.com/cybozu-go/nyallocator/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

var _ = Describe("nyallocator e2e test", func() {
	It("should allocate nodes", func() {
		By("applying NodeTemplate manifest")
		_, _, err := kubectl(nil, "apply", "-f", "./manifests/nodetemplate.yaml")
		Expect(err).NotTo(HaveOccurred())

		By("checking node templates")
		Eventually(func(g Gomega) {
			nt := nyallocatorv1.NodeTemplateList{}
			out, _, err := kubectl(nil, "get", "nodetemplates", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal(out, &nt)
			g.Expect(err).NotTo(HaveOccurred())
			for _, n := range nt.Items {
				g.Expect(n.Status.Sufficient).To(BeTrue())
			}
		}).Should(Succeed(), "Node templates should be sufficient")

		By("checking nodes")
		cp := &corev1.NodeList{}
		cs := &corev1.NodeList{}
		ss := &corev1.NodeList{}

		out, _, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=control-plane", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(out, cp)
		Expect(err).NotTo(HaveOccurred())
		Expect(cp.Items).To(HaveLen(3))
		for _, n := range cp.Items {
			Expect(n.Labels).To(HaveKeyWithValue("node-role.kubernetes.io/control-plane", "true"))
		}
		out, _, err = kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(out, cs)
		Expect(err).NotTo(HaveOccurred())
		Expect(cs.Items).To(HaveLen(2))
		for _, n := range cs.Items {
			Expect(n.Labels).To(HaveKeyWithValue("cke.cybozu.com/role", "cs"))
		}

		out, _, err = kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=ss", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(out, ss)
		Expect(err).NotTo(HaveOccurred())
		Expect(ss.Items).To(HaveLen(2))
		for _, n := range ss.Items {
			Expect(n.Labels).To(HaveKeyWithValue("cke.cybozu.com/role", "ss"))
		}

	})

	It("should allocate node when templates are updated", func() {
		By("updating NodeTemplate manifest")
		nt := &nyallocatorv1.NodeTemplate{}
		out, _, err := kubectl(nil, "get", "nodetemplates", "ss", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(out, nt)
		Expect(err).NotTo(HaveOccurred())
		generation := nt.ObjectMeta.Generation

		nt.Spec.Nodes = 3
		data, err := yaml.Marshal(nt)
		Expect(err).NotTo(HaveOccurred())
		_, _, err = kubectl(data, "apply", "-f", "-")
		Expect(err).NotTo(HaveOccurred())

		By("checking node templates after update")
		Eventually(func(g Gomega) {
			ntList := nyallocatorv1.NodeTemplateList{}
			out, _, err := kubectl(nil, "get", "nodetemplates", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal(out, &ntList)
			g.Expect(err).NotTo(HaveOccurred())
			for _, n := range ntList.Items {
				if n.Name == "ss" {
					g.Expect(n.Status.ReconcileInfo.Generation).To(Equal(generation + 1))
				}
				g.Expect(n.Status.Sufficient).To(BeTrue())
			}
		}).Should(Succeed(), "Node templates should be sufficient after update")
	})

	It("should allocate node when node is deleted", func() {
		By("deleting a node")
		nodes := &corev1.NodeList{}
		out, _, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(out, nodes)
		Expect(err).NotTo(HaveOccurred())
		Expect(nodes.Items).To(HaveLen(2))
		nodeName := nodes.Items[0].Name
		_, _, err = kubectl(nil, "delete", "node", nodeName)
		Expect(err).NotTo(HaveOccurred())

		By("checking nodes")
		Eventually(func(g Gomega) {
			cs := &corev1.NodeList{}
			out, _, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
			g.Expect(err).NotTo(HaveOccurred())
			err = yaml.Unmarshal(out, cs)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(cs.Items).To(HaveLen(2))
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})

	It("should not allow edit of reference labels", func() {
		By("getting a node")
		cs := &corev1.NodeList{}
		out, _, err := kubectl(nil, "get", "nodes", "-l", "nyallocator.cybozu.io/node-template=cs", "-o", "yaml")
		Expect(err).NotTo(HaveOccurred())
		err = yaml.Unmarshal(out, cs)
		Expect(err).NotTo(HaveOccurred())
		nodeName := cs.Items[0].Name

		By("editing a node label")
		_, stderr, err := kubectl(nil, "label", "node", nodeName, "nyallocator.cybozu.io/node-template=foo", "--overwrite")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))

		_, stderr, err = kubectl(nil, "label", "node", nodeName, "nyallocator.cybozu.io/node-template-", "--overwrite")
		Expect(err).To(HaveOccurred())
		Expect(string(stderr)).To(ContainSubstring("ValidatingAdmissionPolicy"))
	})
})
