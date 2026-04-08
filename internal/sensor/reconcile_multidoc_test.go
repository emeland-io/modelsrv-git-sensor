package sensor_test

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

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("reconcile multi-doc behavior", func() {
	It("emits for each resource in a multi-doc file and dedupes unchanged", func() {
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

		st := sensor.NewMemState()
		Expect(sensor.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		Eventually(func() int32 { return pushes.Load() }, 3*time.Second).Should(BeNumerically(">=", 3))

		// Unchanged reconcile: no new pushes
		prev := pushes.Load()
		Expect(sensor.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())
		Consistently(func() int32 { return pushes.Load() }, 200*time.Millisecond).Should(Equal(prev))

		_ = events.CreateOperation
	})

	It("reconciles nested YAML files under a directory", func() {
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

		st := sensor.NewMemState()
		Expect(sensor.ReconcileDir(context.Background(), srv, st, filepath.Join(tmp, "watchedDir"), log)).To(Succeed())

		Eventually(func() int32 { return pushes.Load() }, 3*time.Second).Should(BeNumerically(">=", 1))
	})
})
