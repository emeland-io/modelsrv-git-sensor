package sensor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"

	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("ID extraction", func() {
	It("extracts systemId for System", func() {
		id := uuid.New()
		got, err := sensorTestExtractID(events.SystemResource, map[string]any{"systemId": id.String()})
		Expect(err).NotTo(HaveOccurred())
		Expect(got).To(Equal(id))
	})

	It("errors when required ID is missing", func() {
		_, err := sensorTestExtractID(events.APIResource, map[string]any{})
		Expect(err).To(HaveOccurred())
	})
})

func sensorTestExtractID(rt events.ResourceType, spec map[string]any) (uuid.UUID, error) {
	// shim around unexported function by exercising ReconcileFile’s id parsing indirectly
	// through the exported StableHash shape is overkill; so we call a small exported helper.
	//
	// This is intentionally placed here so we can promote it to an exported helper later if needed.
	return sensor.TestOnlyExtractID(rt, spec)
}

