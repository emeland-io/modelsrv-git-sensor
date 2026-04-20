package reconcile_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/reconcile"
	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

// newSub starts an httptest server that counts POST /api/events/push calls.
func newSub() (*httptest.Server, *atomic.Int32) {
	var pushes atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/events/push" {
			pushes.Add(1)
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	return srv, &pushes
}

var _ = Describe("ReconcileFile integration", func() {
	It("emits Create to subscriber when YAML changes", func() {
		sub, pushes := newSub()
		defer sub.Close()

		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", []string{sub.URL + "/api/"}, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "res.yaml")

		write := func(display string) {
			y := []byte("" +
				"version: emeland.io/test/v1\n" +
				"kind: System\n" +
				"spec:\n" +
				"  systemId: 11111111-1111-1111-1111-111111111111\n" +
				"  displayName: " + display + "\n")
			Expect(os.WriteFile(f, y, 0o644)).To(Succeed())
		}

		st := reconcile.NewState()
		ctx := context.Background()

		write("A")
		Expect(reconcile.ReconcileFile(ctx, srv, st, f, log)).To(Succeed())
		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">=", 1))

		// Same content: should not re-push.
		prev := pushes.Load()
		Expect(reconcile.ReconcileFile(ctx, srv, st, f, log)).To(Succeed())
		Consistently(func() int32 { return pushes.Load() }).Should(Equal(prev))

		// Change YAML -> should push again (Update op).
		write("B")
		Expect(reconcile.ReconcileFile(ctx, srv, st, f, log)).To(Succeed())
		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">", prev))
	})

	It("replays past events to a subscriber registered later", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "res.yaml")
		y := []byte("" +
			"version: emeland.io/test/v1\n" +
			"kind: System\n" +
			"spec:\n" +
			"  systemId: 22222222-2222-2222-2222-222222222222\n" +
			"  displayName: X\n")
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		var pushes atomic.Int32
		sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/events/push" {
				pushes.Add(1)
				w.WriteHeader(http.StatusOK)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer sub.Close()

		// No subscriber yet => no pushes.
		Consistently(func() int32 { return pushes.Load() }).Should(Equal(int32(0)))

		// Register subscriber later: should receive replay of the past event.
		srvEvents := srv.EventManager()
		Expect(srvEvents.AddSubscriber(sub.URL + "/api/")).To(Succeed())

		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">=", 1))
	})

	It("accepts ApiInstance wire kind in YAML and emits", func() {
		sub, pushes := newSub()
		defer sub.Close()

		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", []string{sub.URL + "/api/"}, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "apiinst.yaml")
		y := []byte(`---
version: emeland.io/v1
kind: ApiInstance
spec:
  apiInstanceId: 55555555-5555-4555-8555-555555555555
  displayName: "wi"
  apiId: 66666666-6666-4666-8666-666666666666
  systemInstanceId: 77777777-7777-4777-8777-777777777777
`)
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())
		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">=", 1))
	})
})

var _ = Describe("reconcile multi-doc behavior", func() {
	It("emits for each resource in a multi-doc file and dedupes unchanged", func() {
		sub, pushes := newSub()
		defer sub.Close()

		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", []string{sub.URL + "/api/"}, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "many.yaml")
		yaml := `---
version: emeland.io/v1
kind: System
spec:
  systemId: "11111111-1111-4111-8111-111111111111"
  displayName: "one"
  abstract: false
---
version: emeland.io/v1
kind: System
spec:
  systemId: "22222222-2222-4222-8222-222222222222"
  displayName: "two"
  abstract: false
---
version: emeland.io/v1
kind: System
spec:
  systemId: "33333333-3333-4333-8333-333333333333"
  displayName: "three"
  abstract: false
`
		Expect(os.WriteFile(f, []byte(yaml), 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		Eventually(func() int32 { return pushes.Load() }, 3*time.Second).Should(BeNumerically(">=", 3))

		// Unchanged reconcile: no new pushes
		prev := pushes.Load()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())
		Consistently(func() int32 { return pushes.Load() }, 200*time.Millisecond).Should(Equal(prev))

		_ = events.CreateOperation
	})

	It("reconciles nested YAML files under a directory", func() {
		sub, pushes := newSub()
		defer sub.Close()

		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", []string{sub.URL + "/api/"}, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		subDir := filepath.Join(tmp, "watchedDir", "deep")
		Expect(os.MkdirAll(subDir, 0o755)).To(Succeed())
		f := filepath.Join(subDir, "x.yaml")
		y := []byte(`version: emeland.io/v1
kind: System
spec:
  systemId: "44444444-4444-4444-8444-444444444444"
  displayName: "nested"
  abstract: false
`)
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileDir(context.Background(), srv, st, filepath.Join(tmp, "watchedDir"), log)).To(Succeed())

		Eventually(func() int32 { return pushes.Load() }, 3*time.Second).Should(BeNumerically(">=", 1))
	})
})

var _ = Describe("reconcile skip behavior", func() {
	It("logs build event failures as warn and still emits other docs", func() {
		core, logs := observer.New(zap.DebugLevel)
		log := zap.New(core).Sugar()

		sub, pushes := newSub()
		defer sub.Close()

		srv, err := sensor.New("127.0.0.1:0", []string{sub.URL + "/api/"}, zap.NewNop().Sugar())
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "mixed.yaml")
		yaml := `---
version: emeland.io/v1
kind: System
spec:
  systemId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
  displayName: "ok"
  abstract: false
---
version: emeland.io/v1
kind: API
spec:
  apiId: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
  displayName: "bad type"
  type: "AsyncAPI"
  systemId: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
`
		Expect(os.WriteFile(f, []byte(yaml), 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		skips := logs.FilterMessageSnippet("build event failed (skipping)").All()
		Expect(skips).To(HaveLen(1))
		Expect(skips[0].Level).To(Equal(zap.WarnLevel))

		Eventually(func() int32 { return pushes.Load() }, 3*time.Second).Should(BeNumerically(">=", 1))
	})

	It("logs invalid IDs as warn", func() {
		core, logs := observer.New(zap.DebugLevel)
		log := zap.New(core).Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, zap.NewNop().Sugar())
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "badid.yaml")
		yaml := `version: emeland.io/v1
kind: System
spec:
  systemId: "not-a-uuid"
  displayName: "x"
  abstract: false
`
		Expect(os.WriteFile(f, []byte(yaml), 0o644)).To(Succeed())
		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		bad := logs.FilterMessageSnippet("invalid document id (skipping)").All()
		Expect(bad).To(HaveLen(1))
		Expect(bad[0].Level).To(Equal(zap.WarnLevel))
	})
})
