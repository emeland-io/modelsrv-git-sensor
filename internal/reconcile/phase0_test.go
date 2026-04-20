package reconcile_test

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/reconcile"
	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("phase-0 reconcile", func() {
	It("records Context event in master list", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "ctx.yaml")
		y := []byte(`version: emeland.io/v1
kind: Context
spec:
  contextId: "c0000001-0000-4000-8000-000000000001"
  displayName: "Test Context"
`)
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		Eventually(func() []events.Event {
			return srv.MasterEvents()
		}, 2*time.Second).Should(ContainElement(
			WithTransform(func(e events.Event) events.ResourceType { return e.ResourceType },
				Equal(events.ContextResource)),
		))
	})

	It("records ContextType, Node, and NodeType events in master list", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "phase0.yaml")
		y := []byte(`---
version: emeland.io/v1
kind: ContextType
spec:
  contextTypeId: "c0000002-0000-4000-8000-000000000002"
  displayName: "My ContextType"
---
version: emeland.io/v1
kind: Node
spec:
  nodeId: "c0000003-0000-4000-8000-000000000003"
  displayName: "My Node"
---
version: emeland.io/v1
kind: NodeType
spec:
  nodeTypeId: "c0000004-0000-4000-8000-000000000004"
  displayName: "My NodeType"
`)
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		masterTypes := func() []events.ResourceType {
			evs := srv.MasterEvents()
			out := make([]events.ResourceType, len(evs))
			for i, e := range evs {
				out[i] = e.ResourceType
			}
			return out
		}

		Eventually(masterTypes, 2*time.Second).Should(ContainElements(
			events.ContextTypeResource,
			events.NodeResource,
			events.NodeTypeResource,
		))
	})
})

var _ = Describe("upsert behavior", func() {
	It("emits Create on first reconcile and Update on content change", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "upsert.yaml")
		writeSystem := func(name string) {
			y := []byte("version: emeland.io/v1\nkind: System\nspec:\n  systemId: \"d0000001-0000-4000-8000-000000000001\"\n  displayName: \"" + name + "\"\n  abstract: false\n")
			Expect(os.WriteFile(f, y, 0o644)).To(Succeed())
		}

		st := reconcile.NewState()

		writeSystem("first")
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		Eventually(func() []events.Event { return srv.MasterEvents() }, 2*time.Second).
			Should(ContainElement(
				And(
					WithTransform(func(e events.Event) events.Operation { return e.Operation }, Equal(events.CreateOperation)),
					WithTransform(func(e events.Event) events.ResourceType { return e.ResourceType }, Equal(events.SystemResource)),
				),
			))

		// Change content: should emit Update, not Create.
		writeSystem("second")
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		Eventually(func() []events.Event { return srv.MasterEvents() }, 2*time.Second).
			Should(ContainElement(
				And(
					WithTransform(func(e events.Event) events.Operation { return e.Operation }, Equal(events.UpdateOperation)),
					WithTransform(func(e events.Event) events.ResourceType { return e.ResourceType }, Equal(events.SystemResource)),
				),
			))
	})
})

var _ = Describe("delete behavior", func() {
	It("emits Delete when a document is removed from a file", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "delete_doc.yaml")

		twoSystems := []byte(`---
version: emeland.io/v1
kind: System
spec:
  systemId: "e0000001-0000-4000-8000-000000000001"
  displayName: "keep"
  abstract: false
---
version: emeland.io/v1
kind: System
spec:
  systemId: "e0000002-0000-4000-8000-000000000002"
  displayName: "remove"
  abstract: false
`)
		Expect(os.WriteFile(f, twoSystems, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		// Remove the second system.
		oneSystem := []byte(`version: emeland.io/v1
kind: System
spec:
  systemId: "e0000001-0000-4000-8000-000000000001"
  displayName: "keep"
  abstract: false
`)
		Expect(os.WriteFile(f, oneSystem, 0o644)).To(Succeed())
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())

		deletedID := uuid.MustParse("e0000002-0000-4000-8000-000000000002")
		Eventually(func() []events.Event { return srv.MasterEvents() }, 2*time.Second).
			Should(ContainElement(
				And(
					WithTransform(func(e events.Event) events.Operation { return e.Operation }, Equal(events.DeleteOperation)),
					WithTransform(func(e events.Event) uuid.UUID { return e.ResourceId }, Equal(deletedID)),
				),
			))
	})

	It("emits Delete for all resources when a file disappears from a directory", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "vanish.yaml")
		y := []byte(`version: emeland.io/v1
kind: System
spec:
  systemId: "f0000001-0000-4000-8000-000000000001"
  displayName: "will vanish"
  abstract: false
`)
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileDir(context.Background(), srv, st, tmp, log)).To(Succeed())

		// Delete the file; next dir reconcile should emit Delete.
		Expect(os.Remove(f)).To(Succeed())
		Expect(reconcile.ReconcileDir(context.Background(), srv, st, tmp, log)).To(Succeed())

		vanishedID := uuid.MustParse("f0000001-0000-4000-8000-000000000001")
		Eventually(func() []events.Event { return srv.MasterEvents() }, 2*time.Second).
			Should(ContainElement(
				And(
					WithTransform(func(e events.Event) events.Operation { return e.Operation }, Equal(events.DeleteOperation)),
					WithTransform(func(e events.Event) uuid.UUID { return e.ResourceId }, Equal(vanishedID)),
				),
			))
	})

	It("does not emit Delete for unchanged documents on re-reconcile", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		tmp := GinkgoT().TempDir()
		f := filepath.Join(tmp, "stable.yaml")
		y := []byte(`version: emeland.io/v1
kind: System
spec:
  systemId: "f0000002-0000-4000-8000-000000000002"
  displayName: "stable"
  abstract: false
`)
		Expect(os.WriteFile(f, y, 0o644)).To(Succeed())

		st := reconcile.NewState()
		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())
		before := len(srv.MasterEvents())

		Expect(reconcile.ReconcileFile(context.Background(), srv, st, f, log)).To(Succeed())
		Consistently(func() int { return len(srv.MasterEvents()) }, 200*time.Millisecond).
			Should(Equal(before))
	})
})
