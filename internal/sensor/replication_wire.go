package sensor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.emeland.io/modelsrv/pkg/events"
)

// cloneEventForSubscriberNotify copies the event and its Objects slice so Notify can
// reshape the payload without mutating events stored in the master list.
func cloneEventForSubscriberNotify(src *events.Event) events.Event {
	if src == nil {
		return events.Event{}
	}
	out := *src
	if n := len(src.Objects); n > 0 {
		out.Objects = make([]any, n)
		copy(out.Objects, src.Objects)
	}
	return out
}

// patchWirePayloadForReplication maps domain-encoded JSON into OpenAPI wire shapes expected by
// ModelSrv replication (POST /events/push). See PushWireEventFromDomain and replication_decode.
func patchWirePayloadForReplication(ev *events.Event) {
	if ev == nil || len(ev.Objects) == 0 {
		return
	}
	b, err := json.Marshal(ev.Objects[0])
	if err != nil {
		return
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		return
	}
	rid := ev.ResourceId
	switch ev.ResourceType {
	case events.APIInstanceResource:
		setWireInstanceID(m, "apiInstanceId", []string{"instanceId", "InstanceId", "APIInstanceId"}, rid)
	case events.ComponentInstanceResource:
		setWireInstanceID(m, "componentInstanceId", []string{"instanceId", "InstanceId", "ComponentInstanceId"}, rid)
		normalizeWireUUIDSliceFields(m, "consumes", "provides")
	case events.SystemInstanceResource:
		setWireInstanceID(m, "systemInstanceId", []string{"instanceId", "InstanceId", "SystemInstanceId"}, rid)
	case events.ComponentResource:
		normalizeWireUUIDSliceFields(m, "consumes", "provides")
	default:
		return
	}
	ev.Objects = []any{m}
}

func setWireInstanceID(m map[string]interface{}, canonical string, aliases []string, fallback uuid.UUID) {
	if scalarNonEmpty(m[canonical]) {
		return
	}
	for _, k := range aliases {
		if scalarNonEmpty(m[k]) {
			m[canonical] = m[k]
			return
		}
	}
	if fallback != uuid.Nil {
		m[canonical] = fallback.String()
	}
}

// normalizeWireUUIDSliceFields replaces consumes/provides-style slices of ref objects with
// plain UUID strings, matching OpenAPI *[]openapi_types.UUID.
func normalizeWireUUIDSliceFields(m map[string]interface{}, fields ...string) {
	for _, f := range fields {
		want := strings.ToLower(f)
		raw := mapValueFold(m, want)
		if raw == nil {
			continue
		}
		sl := uuidSliceFromWireRefs(raw)
		deleteKeyFold(m, want)
		if len(sl) > 0 {
			m[want] = sl
		}
	}
}

func mapValueFold(m map[string]interface{}, wantLower string) interface{} {
	for k, v := range m {
		if strings.EqualFold(k, wantLower) {
			return v
		}
	}
	return nil
}

func deleteKeyFold(m map[string]interface{}, wantLower string) {
	for k := range m {
		if strings.EqualFold(k, wantLower) {
			delete(m, k)
		}
	}
}

func uuidSliceFromWireRefs(raw interface{}) []interface{} {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]interface{}, 0, len(arr))
	for _, el := range arr {
		if s := extractUUIDScalar(el); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func extractUUIDScalar(el interface{}) string {
	if el == nil {
		return ""
	}
	switch t := el.(type) {
	case string:
		s := strings.TrimSpace(t)
		if _, err := uuid.Parse(s); err == nil {
			return s
		}
	case map[string]interface{}:
		for _, k := range []string{
			"apiId", "ApiId", "APIId", "apiID",
			"apiInstanceId", "ApiInstanceId", "APIInstanceId",
			"resourceId", "ResourceId",
			"systemId", "SystemId",
			"componentId", "ComponentId",
			"id", "uuid",
		} {
			if v, ok := t[k]; ok {
				if s := extractUUIDScalar(v); s != "" {
					return s
				}
			}
		}
		// Nested ref: { "Api": { "apiId": "..." } }
		for _, nest := range []string{"Api", "api", "API", "System", "system", "Component", "component"} {
			if v, ok := t[nest]; ok {
				if s := extractUUIDScalar(v); s != "" {
					return s
				}
			}
		}
	case json.Number:
		return strings.TrimSpace(t.String())
	case float64:
		// JSON numbers for UUID unlikely; skip
	}
	return ""
}

func scalarNonEmpty(v interface{}) bool {
	if v == nil {
		return false
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if s == "" || s == "00000000-0000-0000-0000-000000000000" {
		return false
	}
	return true
}
