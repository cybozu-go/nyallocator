package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Utilities functions tests", func() {
	Context("sortTaints function", func() {
		It("should sort taints correctly", func() {
			taints := []corev1.Taint{
				{Key: "b", Value: "2", Effect: corev1.TaintEffectNoSchedule},
				{Key: "a", Value: "2", Effect: corev1.TaintEffectNoSchedule},
				{Key: "a", Value: "1", Effect: corev1.TaintEffectPreferNoSchedule},
				{Key: "a", Value: "1", Effect: corev1.TaintEffectNoSchedule},
			}
			sorted := sortTaints(taints)
			expected := []corev1.Taint{
				{Key: "a", Value: "1", Effect: corev1.TaintEffectNoSchedule},
				{Key: "a", Value: "1", Effect: corev1.TaintEffectPreferNoSchedule},
				{Key: "a", Value: "2", Effect: corev1.TaintEffectNoSchedule},
				{Key: "b", Value: "2", Effect: corev1.TaintEffectNoSchedule},
			}
			Expect(sorted).To(Equal(expected))
		})
	})
	Context("taintsEqual function", func() {
		It("should return true for equal taint slices", func() {
			taints1 := []corev1.Taint{
				{Key: "key1", Value: "value1", Effect: corev1.TaintEffectNoSchedule},
				{Key: "key2", Value: "value2", Effect: corev1.TaintEffectPreferNoSchedule},
			}
			taints2 := []corev1.Taint{
				{Key: "key2", Value: "value2", Effect: corev1.TaintEffectPreferNoSchedule},
				{Key: "key1", Value: "value1", Effect: corev1.TaintEffectNoSchedule},
			}
			Expect(taintsEqual(taints1, taints2)).To(BeTrue())
		})
	})
})
