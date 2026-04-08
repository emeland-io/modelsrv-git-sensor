package sensor_test

import (
	"bytes"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("doc decoding", func() {
	It("accepts ApiInstance wire kind", func() {
		yaml := `---
version: emeland.io/v1
kind: System
spec:
  systemId: "e1000001-0000-4000-8000-000000000001"
  displayName: "x"
  abstract: false
---
version: emeland.io/v1
kind: ApiInstance
spec:
  apiInstanceId: "e1000001-0000-4000-8000-000000000012"
  displayName: "y"
  apiId: "e1000001-0000-4000-8000-000000000002"
  systemInstanceId: "e1000001-0000-4000-8000-000000000011"
`
		docs, err := sensor.TestOnlyDecodeDocuments([]byte(yaml))
		Expect(err).NotTo(HaveOccurred())
		Expect(docs).To(HaveLen(2))
		Expect(docs[1].Kind.ResourceType()).To(Equal(events.APIInstanceResource))
	})

	It("decodes multiple ApiInstance documents", func() {
		yaml := `---
version: emeland.io/v1
kind: ApiInstance
spec:
  apiInstanceId: "e1000001-0000-4000-8000-000000000012"
  displayName: "a"
  apiId: "e1000001-0000-4000-8000-000000000002"
  systemInstanceId: "e1000001-0000-4000-8000-000000000011"
---
version: emeland.io/v1
kind: ApiInstance
spec:
  apiInstanceId: "e1000001-0000-4000-8000-000000000013"
  displayName: "b"
  apiId: "e1000001-0000-4000-8000-000000000003"
  systemInstanceId: "e1000001-0000-4000-8000-000000000011"
`
		docs, err := sensor.TestOnlyDecodeDocuments([]byte(yaml))
		Expect(err).NotTo(HaveOccurred())
		Expect(docs).To(HaveLen(2))
		for i := range docs {
			Expect(docs[i].Kind.ResourceType()).To(Equal(events.APIInstanceResource), "doc %d", i)
		}
	})

	It("normalizes indented kind line", func() {
		in := []byte("version: emeland.io/v1\n  kind: ApiInstance\nspec: {}\n")
		out := sensor.TestOnlyNormalizeYAMLKindsForFileSensor(in)
		Expect(bytes.Contains(out, []byte("kind: APIInstance"))).To(BeTrue(), "got %q", out)
	})
})
