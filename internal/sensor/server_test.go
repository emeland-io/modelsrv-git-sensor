package sensor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("sensor self-registration", func() {
	It("emits NodeType Create and Node Create events on startup", func() {
		log := zap.NewNop().Sugar()
		srv, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = srv.Close() }()

		master := srv.MasterEvents()

		// Exactly two registration events are emitted before any YAML reconcile runs.
		Expect(master).To(HaveLen(2))

		// First event: NodeType Create carrying the hardcoded binary UUID.
		Expect(master[0].ResourceType).To(Equal(events.NodeTypeResource))
		Expect(master[0].Operation).To(Equal(events.CreateOperation))
		Expect(master[0].ResourceId).To(Equal(sensor.ExportNodeTypeID()))

		// Second event: Node Create with a non-nil per-process UUID.
		Expect(master[1].ResourceType).To(Equal(events.NodeResource))
		Expect(master[1].Operation).To(Equal(events.CreateOperation))
		Expect(master[1].ResourceId).NotTo(Equal(uuid.Nil))

		// The node UUID exposed via test helper matches the event.
		nodeID := sensor.ExportNodeID(srv)
		Expect(nodeID).NotTo(Equal(uuid.Nil))
		Expect(nodeID).To(Equal(master[1].ResourceId))
	})

	It("generates a different nodeID on each startup", func() {
		log := zap.NewNop().Sugar()

		srv1, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		id1 := sensor.ExportNodeID(srv1)
		_ = srv1.Close()

		srv2, err := sensor.New("127.0.0.1:0", nil, log)
		Expect(err).NotTo(HaveOccurred())
		id2 := sensor.ExportNodeID(srv2)
		_ = srv2.Close()

		Expect(id1).NotTo(Equal(uuid.Nil))
		Expect(id2).NotTo(Equal(uuid.Nil))
		Expect(id1).NotTo(Equal(id2))
	})
})
