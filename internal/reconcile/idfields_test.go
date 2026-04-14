package reconcile_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/reconcile"
)

var _ = Describe("ID extraction", func() {
	It("extracts systemId for System", func() {
		id := uuid.New()
		got, err := reconcile.ExportExtractID(events.SystemResource, map[string]any{"systemId": id.String()})
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(id))
	})

	It("errors when required ID is missing", func() {
		_, err := reconcile.ExportExtractID(events.APIResource, map[string]any{})
		Expect(err).To(HaveOccurred())
	})

	DescribeTable("phase-0 ID extraction",
		func(rt events.ResourceType, field string) {
			id := uuid.New()
			got, err := reconcile.ExportExtractID(rt, map[string]any{field: id.String()})
			Expect(err).NotTo(HaveOccurred())
			Expect(got).To(Equal(id))
		},
		Entry("Context", events.ContextResource, "contextId"),
		Entry("ContextType", events.ContextTypeResource, "contextTypeId"),
		Entry("Node", events.NodeResource, "nodeId"),
		Entry("NodeType", events.NodeTypeResource, "nodeTypeId"),
	)
})
