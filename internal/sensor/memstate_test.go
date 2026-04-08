package sensor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("MemState", func() {
	It("stores and retrieves values", func() {
		st := sensor.NewMemState()
		_, ok := st.Get("k")
		Expect(ok).To(BeFalse())

		st.Set("k", "h")
		v, ok := st.Get("k")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("h"))
	})

	It("does not panic on nil receiver Set", func() {
		var st *sensor.MemState
		Expect(func() { st.Set("k", "v") }).NotTo(Panic())
	})
})
