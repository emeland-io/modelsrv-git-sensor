package sensor_test

import (
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.emeland.io/modelsrv/pkg/events"

	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

var _ = Describe("replication wire", func() {
	uid := func() uuid.UUID {
		u, err := uuid.Parse("3fa85f64-5717-4562-b3fc-2c963f66afa6")
		Expect(err).NotTo(HaveOccurred())
		return u
	}
	other := func() uuid.UUID {
		u, err := uuid.Parse("11111111-1111-1111-1111-111111111111")
		Expect(err).NotTo(HaveOccurred())
		return u
	}

	Describe("ExportCloneEventForSubscriberNotify", func() {
		It("returns an empty event for a nil source", func() {
			out := sensor.ExportCloneEventForSubscriberNotify(nil)
			Expect(out).To(Equal(events.Event{}))
		})

		It("copies the Objects slice but shares element values; replacing the clone slice does not affect the source", func() {
			m := map[string]any{"k": 1}
			src := &events.Event{ResourceType: events.APIInstanceResource, Objects: []any{m}}
			out := sensor.ExportCloneEventForSubscriberNotify(src)
			Expect(out.Objects).To(HaveLen(1))
			Expect(out.Objects).NotTo(BeIdenticalTo(src.Objects))
			// Shared map: in-place change visible on both.
			out0 := out.Objects[0].(map[string]any)
			out0["k"] = 99
			Expect(src.Objects[0].(map[string]any)["k"]).To(Equal(99))
			// Replacing the clone's slice only does not change the source.
			newMap := map[string]any{"k": 2}
			out.Objects = []any{newMap}
			Expect(src.Objects).To(HaveLen(1))
			Expect(src.Objects[0].(map[string]any)["k"]).To(Equal(99))
		})
	})

	Describe("ExportPatchWirePayloadForReplication", func() {
		It("is a no-op for nil or empty objects", func() {
			sensor.ExportPatchWirePayloadForReplication(nil)
			ev := &events.Event{ResourceType: events.APIInstanceResource, ResourceId: uid()}
			sensor.ExportPatchWirePayloadForReplication(ev)
			Expect(ev.Objects).To(BeNil())
		})

		It("leaves the payload unchanged for an unsupported resource type", func() {
			m := map[string]any{"foo": "bar"}
			ev := &events.Event{ResourceType: events.NodeResource, ResourceId: uid(), Objects: []any{m}}
			sensor.ExportPatchWirePayloadForReplication(ev)
			Expect(ev.Objects).To(HaveLen(1))
			Expect(ev.Objects[0]).To(Equal(m))
		})

		It("maps API instance fields and respects canonical apiInstanceId", func() {
			rid := other()
			ev := &events.Event{
				ResourceType: events.APIInstanceResource,
				ResourceId:   rid,
				Objects:      []any{map[string]any{"instanceId": rid.String()}},
			}
			sensor.ExportPatchWirePayloadForReplication(ev)
			obj, ok := ev.Objects[0].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(obj["apiInstanceId"]).To(Equal(rid.String()))
		})

		It("fills apiInstanceId from ResourceId when absent", func() {
			rid := uid()
			ev := &events.Event{ResourceType: events.APIInstanceResource, ResourceId: rid, Objects: []any{map[string]any{}}}
			sensor.ExportPatchWirePayloadForReplication(ev)
			obj := ev.Objects[0].(map[string]any)
			Expect(obj["apiInstanceId"]).To(Equal(rid.String()))
		})

		It("sets componentInstanceId and flattens consumes/provides", func() {
			a := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
			b := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
			rid := uid()
			ev := &events.Event{
				ResourceType: events.ComponentInstanceResource,
				ResourceId:   rid,
				Objects: []any{map[string]any{
					"InstanceId": rid.String(),
					"Consumes": []any{
						map[string]any{"apiId": a},
					},
					"provides": []any{b},
				}},
			}
			sensor.ExportPatchWirePayloadForReplication(ev)
			obj := ev.Objects[0].(map[string]any)
			Expect(obj["componentInstanceId"]).To(Equal(rid.String()))
			Expect(obj["consumes"]).To(Equal([]any{a}))
			Expect(obj["provides"]).To(Equal([]any{b}))
			_, hasConsumes := obj["Consumes"]
			Expect(hasConsumes).To(BeFalse())
		})

		It("sets systemInstanceId for system instance resources", func() {
			rid := uid()
			ev := &events.Event{
				ResourceType: events.SystemInstanceResource,
				ResourceId:   rid,
				Objects:      []any{map[string]any{"SystemInstanceId": rid.String()}},
			}
			sensor.ExportPatchWirePayloadForReplication(ev)
			obj := ev.Objects[0].(map[string]any)
			Expect(obj["systemInstanceId"]).To(Equal(rid.String()))
		})

		It("normalizes consumes and provides for component resource without instance id fields", func() {
			c := "cccccccc-cccc-cccc-cccc-cccccccccccc"
			ev := &events.Event{
				ResourceType: events.ComponentResource,
				Objects: []any{map[string]any{
					"Provides": []any{map[string]any{"ComponentId": c}},
				}},
			}
			sensor.ExportPatchWirePayloadForReplication(ev)
			obj := ev.Objects[0].(map[string]any)
			Expect(obj["provides"]).To(Equal([]any{c}))
		})
	})

	Describe("ExportSetWireInstanceID", func() {
		It("prefers the canonical key when set", func() {
			m := map[string]any{"id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "ID": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"}
			fb := other()
			sensor.ExportSetWireInstanceID(m, "id", []string{"ID"}, fb)
			Expect(m["id"]).To(Equal("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"))
		})

		It("copies from the first matching alias", func() {
			m := map[string]any{"InstanceId": "cccccccc-cccc-cccc-cccc-cccccccccccc"}
			fb := uid()
			sensor.ExportSetWireInstanceID(m, "componentInstanceId", []string{"instanceId", "InstanceId"}, fb)
			Expect(m["componentInstanceId"]).To(Equal("cccccccc-cccc-cccc-cccc-cccccccccccc"))
		})

		It("uses the fallback when canonical and aliases are empty", func() {
			m := map[string]any{}
			fb := uid()
			sensor.ExportSetWireInstanceID(m, "apiInstanceId", []string{"x"}, fb)
			Expect(m["apiInstanceId"]).To(Equal(fb.String()))
		})

		It("does not set fallback when it is nil UUID", func() {
			m := map[string]any{}
			sensor.ExportSetWireInstanceID(m, "apiInstanceId", []string{}, uuid.Nil)
			_, ok := m["apiInstanceId"]
			Expect(ok).To(BeFalse())
		})
	})

	Describe("ExportNormalizeWireUUIDSliceFields", func() {
		It("replaces mixed-case keys with lowercase wire keys holding UUID strings", func() {
			u := "dddddddd-dddd-dddd-dddd-dddddddddddd"
			m := map[string]any{"CONSUMES": []any{map[string]any{"id": u}}}
			sensor.ExportNormalizeWireUUIDSliceFields(m, "consumes")
			_, hasOld := m["CONSUMES"]
			Expect(hasOld).To(BeFalse())
			Expect(m["consumes"]).To(Equal([]any{u}))
		})

		It("drops empty results without adding an empty key", func() {
			m := map[string]any{"consumes": []any{map[string]any{"nope": "x"}}}
			sensor.ExportNormalizeWireUUIDSliceFields(m, "consumes")
			_, has := m["consumes"]
			Expect(has).To(BeFalse())
		})
	})

	Describe("map key folding helpers", func() {
		It("ExportMapValueFold is case-insensitive", func() {
			m := map[string]any{"Foo": 42}
			Expect(sensor.ExportMapValueFold(m, "foo")).To(Equal(42))
		})

		It("ExportDeleteKeyFold removes keys case-insensitively", func() {
			m := map[string]any{"Bar": 1, "baz": 2}
			sensor.ExportDeleteKeyFold(m, "bar")
			Expect(m).To(Equal(map[string]any{"baz": 2}))
		})
	})

	Describe("ExportUUIDSliceFromWireRefs", func() {
		It("returns nil for non-slice input", func() {
			Expect(sensor.ExportUUIDSliceFromWireRefs("nope")).To(BeNil())
		})
	})

	Describe("ExportExtractUUIDScalar", func() {
		It("accepts valid UUID strings", func() {
			s := "550e8400-e29b-41d4-a716-446655440000"
			Expect(sensor.ExportExtractUUIDScalar(s)).To(Equal(s))
		})

		It("rejects non-UUID strings", func() {
			Expect(sensor.ExportExtractUUIDScalar("not-a-uuid")).To(BeEmpty())
		})

		It("extracts from nested object keys", func() {
			m := map[string]any{"Api": map[string]any{"apiId": "660e8400-e29b-41d4-a716-446655440000"}}
			Expect(sensor.ExportExtractUUIDScalar(m)).To(Equal("660e8400-e29b-41d4-a716-446655440000"))
		})

		It("ignores float64 (JSON number form from Unmarshal, not a wire UUID form)", func() {
			Expect(sensor.ExportExtractUUIDScalar(float64(42))).To(BeEmpty())
		})
	})

	Describe("ExportScalarNonEmpty", func() {
		It("treats nil, blank, and zero UUID as empty", func() {
			Expect(sensor.ExportScalarNonEmpty(nil)).To(BeFalse())
			Expect(sensor.ExportScalarNonEmpty("")).To(BeFalse())
			Expect(sensor.ExportScalarNonEmpty("  ")).To(BeFalse())
			Expect(sensor.ExportScalarNonEmpty("00000000-0000-0000-0000-000000000000")).To(BeFalse())
		})

		It("treats other scalars as non-empty", func() {
			Expect(sensor.ExportScalarNonEmpty("00000000-0000-0000-0000-000000000001")).To(BeTrue())
		})
	})
})
