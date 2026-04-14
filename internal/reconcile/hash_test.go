package reconcile_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"emeland.io/modelsrv-git-sensor/internal/reconcile"
)

var _ = Describe("StableHash", func() {
	It("is deterministic for map key order", func() {
		a := map[string]any{"b": 2, "a": 1, "nested": map[string]any{"y": "Y", "x": "X"}}
		b := map[string]any{"a": 1, "b": 2, "nested": map[string]any{"x": "X", "y": "Y"}}

		ha, err := reconcile.StableHash(a)
		Expect(err).NotTo(HaveOccurred())
		hb, err := reconcile.StableHash(b)
		Expect(err).NotTo(HaveOccurred())
		Expect(ha).To(Equal(hb))
	})
})
