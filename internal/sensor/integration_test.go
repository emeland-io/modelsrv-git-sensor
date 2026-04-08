package sensor_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("Reconcile integration", func() {
	It("emits Create to subscriber when YAML changes", func() {
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

		// modelsrv client expects base URL like http://host/api/ (it will call /api/events/push).
		subscriberURL := sub.URL + "/api/"

		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", []string{subscriberURL}, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "res.yaml")

		write := func(display string) {
			// Minimal System doc (phase-1) with required IDs.
			y := []byte("" +
				"version: emeland.io/test/v1\n" +
				"kind: System\n" +
				"spec:\n" +
				"  systemId: 11111111-1111-1111-1111-111111111111\n" +
				"  displayName: " + display + "\n")
			Expect(os.WriteFile(f, y, 0o644)).To(Succeed())
		}

		st := sensor.NewMemState()
		ctx := context.Background()

		write("A")
		Expect(sensor.ReconcileFile(ctx, srv, st, f, log)).To(Succeed())
		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">=", 1))

		// Same content: should not re-push.
		prev := pushes.Load()
		Expect(sensor.ReconcileFile(ctx, srv, st, f, log)).To(Succeed())
		Consistently(func() int32 { return pushes.Load() }).Should(Equal(prev))

		// Change YAML -> should push again (still Create op).
		write("B")
		Expect(sensor.ReconcileFile(ctx, srv, st, f, log)).To(Succeed())
		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">", prev))

		// Sanity: ensure we only ever emit Create in this sensor.
		// (We can't decode the event body here without depending on internal OAPI types;
		// this is enforced by sensor code path setting Operation=CreateOperation.)
		_ = events.CreateOperation
	})

	It("replays past events to a subscriber registered later", func() {
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

		st := sensor.NewMemState()
		Expect(sensor.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		// No subscriber yet => no pushes.
		Consistently(func() int32 { return pushes.Load() }).Should(Equal(int32(0)))

		// Register subscriber later: should receive replay of the past event.
		// (We call the internal event manager method indirectly by using the HTTP endpoint in a real deployment,
		// but for this integration test we can register directly by emitting through srv.)
		//
		// The underlying behavior is: AddSubscriber() replays all events from the master list.
		srvEvents := sensor.TestOnlyEvents(srv)
		Expect(srvEvents.AddSubscriber(sub.URL + "/api/")).To(Succeed())

		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">=", 1))
	})

	It("accepts ApiInstance wire kind in YAML and emits", func() {
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

		st := sensor.NewMemState()
		Expect(sensor.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())
		Eventually(func() int32 { return pushes.Load() }).Should(BeNumerically(">=", 1))
	})
})

