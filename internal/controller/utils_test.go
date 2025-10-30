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
		It("should handle nil taint slice", func() {
			sorted := sortTaints(nil)
			Expect(sorted).To(BeEmpty())
		})
		It("should handle empty taint slice", func() {
			sorted := sortTaints([]corev1.Taint{})
			Expect(sorted).To(BeEmpty())
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
		It("should return false for different taint slices", func() {
			taints1 := []corev1.Taint{
				{Key: "key1", Value: "value1", Effect: corev1.TaintEffectNoSchedule},
			}
			taints2 := []corev1.Taint{
				{Key: "key1", Value: "value2", Effect: corev1.TaintEffectNoSchedule},
			}
			taints3 := []corev1.Taint{
				{Key: "key2", Value: "value1", Effect: corev1.TaintEffectNoSchedule},
			}
			taints4 := []corev1.Taint{
				{Key: "key1", Value: "value1", Effect: corev1.TaintEffectPreferNoSchedule},
			}
			Expect(taintsEqual(taints1, taints2)).To(BeFalse())
			Expect(taintsEqual(taints1, taints3)).To(BeFalse())
			Expect(taintsEqual(taints1, taints4)).To(BeFalse())
		})
		It("should return true for empty and nil taint slices", func() {
			Expect(taintsEqual([]corev1.Taint{}, []corev1.Taint{})).To(BeTrue())
			Expect(taintsEqual(nil, nil)).To(BeTrue())
			Expect(taintsEqual([]corev1.Taint{}, nil)).To(BeTrue())
		})
	})
})
