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
	"go.uber.org/zap/zaptest/observer"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("reconcile skip behavior", func() {
	// Ensures modelsrv validation failures on one document are Warn (not Error), so
	// development loggers do not attach stack traces, and other docs still process.
	It("logs build event failures as warn and still emits other docs", func() {
		core, logs := observer.New(zap.DebugLevel)
		log := zap.New(core).Sugar()

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

		st := sensor.NewMemState()
		Expect(sensor.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

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
		st := sensor.NewMemState()
		Expect(sensor.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		bad := logs.FilterMessageSnippet("invalid document id (skipping)").All()
		Expect(bad).To(HaveLen(1))
		Expect(bad[0].Level).To(Equal(zap.WarnLevel))
	})
})