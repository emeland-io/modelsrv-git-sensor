package reconcile_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"emeland.io/modelsrv-git-sensor/internal/reconcile"
)

var _ = Describe("State", func() {
	It("stores and retrieves values", func() {
		st := reconcile.NewState()
		_, ok := st.Get("k")
		Expect(ok).To(BeFalse())

		st.Set("k", "h")
		v, ok := st.Get("k")
		Expect(ok).To(BeTrue())
		Expect(v).To(Equal("h"))
	})

	It("does not panic on nil receiver Set", func() {
		var st *reconcile.State
		Expect(func() { st.Set("k", "v") }).NotTo(Panic())
	})

	It("KeysForPath returns nothing before any registration", func() {
		st := reconcile.NewState()
		Expect(st.KeysForPath("/some/path.yaml")).To(BeEmpty())
	})

	It("PurgePath returns and removes keys", func() {
		st := reconcile.NewState()
		st.Set("System/aaa", "h1")
		st.SetKeysForPath("/f.yaml", map[string]struct{}{"System/aaa": {}})
		keys := st.PurgePath("/f.yaml")
		Expect(keys).To(ConsistOf("System/aaa"))
		_, ok := st.Get("System/aaa")
		Expect(ok).To(BeFalse())
		Expect(st.KeysForPath("/f.yaml")).To(BeEmpty())
	})

	It("PathsUnderDir returns only paths beneath the given directory", func() {
		st := reconcile.NewState()
		st.SetKeysForPath("/watch/a.yaml", map[string]struct{}{"System/aaa": {}})
		st.SetKeysForPath("/watch/sub/b.yaml", map[string]struct{}{"System/bbb": {}})
		st.SetKeysForPath("/other/c.yaml", map[string]struct{}{"System/ccc": {}})
		paths := st.PathsUnderDir("/watch")
		Expect(paths).To(ConsistOf("/watch/a.yaml", "/watch/sub/b.yaml"))
	})
})
