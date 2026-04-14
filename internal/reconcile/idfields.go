package reconcile

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.emeland.io/modelsrv/pkg/events"
	"go.emeland.io/modelsrv/pkg/filesensor"
	"go.emeland.io/modelsrv/pkg/model"
)

// idFields maps each supported resource type to the spec field name(s) that
// contain the resource UUID. This is the single source of truth: supported()
// checks membership, extractID() reads the field names. Adding a new resource
// type is a one-line addition here.
var idFields = map[events.ResourceType][]string{
	// Phase 0
	events.ContextResource:     {"contextId"},
	events.ContextTypeResource: {"contextTypeId"},
	events.NodeResource:        {"nodeId"},
	events.NodeTypeResource:    {"nodeTypeId"},

	// Phase 1
	events.SystemResource:            {"systemId"},
	events.SystemInstanceResource:    {"systemInstanceId", "instanceId"},
	events.APIResource:               {"apiId"},
	events.APIInstanceResource:       {"apiInstanceId", "instanceId"},
	events.ComponentResource:         {"componentId"},
	events.ComponentInstanceResource: {"componentInstanceId", "instanceId"},

	// Phase 5
	events.FindingResource:     {"findingId"},
	events.FindingTypeResource: {"findingTypeId"},
}

func supported(rt events.ResourceType) bool {
	_, ok := idFields[rt]
	return ok
}

func extractID(rt events.ResourceType, spec map[string]any) (uuid.UUID, error) {
	fields, ok := idFields[rt]
	if !ok {
		return uuid.Nil, fmt.Errorf("unsupported resource type %s", rt.String())
	}
	return uuidFromFirst(spec, fields...)
}

// parseKey reverses the "ResourceType/UUID" key format used by State.
func parseKey(key string) (events.ResourceType, uuid.UUID, error) {
	slash := strings.LastIndex(key, "/")
	if slash < 0 {
		return 0, uuid.Nil, fmt.Errorf("invalid key format %q", key)
	}
	rt := events.ParseResourceType(key[:slash])
	if rt == events.UnknownResourceType {
		return 0, uuid.Nil, fmt.Errorf("unknown resource type in key %q", key)
	}
	id, err := uuid.Parse(key[slash+1:])
	if err != nil {
		return 0, uuid.Nil, fmt.Errorf("invalid UUID in key %q: %w", key, err)
	}
	return rt, id, nil
}

func uuidFromFirst(spec map[string]any, keys ...string) (uuid.UUID, error) {
	for _, k := range keys {
		if id, err := uuidFromSpec(spec, k); err == nil {
			return id, nil
		}
	}
	return uuid.Nil, fmt.Errorf("missing required id in one of: %s", strings.Join(keys, ", "))
}

func uuidFromSpec(spec map[string]any, key string) (uuid.UUID, error) {
	raw, ok := spec[key]
	if !ok || raw == nil {
		return uuid.Nil, fmt.Errorf("missing %q", key)
	}
	s, ok := raw.(string)
	if !ok {
		return uuid.Nil, fmt.Errorf("%q must be a string UUID", key)
	}
	id, err := uuid.Parse(strings.TrimSpace(s))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%q: invalid UUID", key)
	}
	return id, nil
}

// buildEvent applies a decoded YAML document into a temporary in-memory model
// and returns the resulting event with its operation field unset (caller sets it).
func buildEvent(doc filesensor.Document) (events.Event, error) {
	sink := &listSink{}
	m, err := model.NewModel(sink)
	if err != nil {
		return events.Event{}, err
	}
	if err := filesensor.ApplyDocument(doc, m); err != nil {
		return events.Event{}, err
	}
	if len(sink.events) == 0 {
		return events.Event{}, fmt.Errorf("no event produced")
	}
	// Take the last event (most specific).
	return sink.events[len(sink.events)-1], nil
}

type listSink struct {
	events []events.Event
}

var _ events.EventSink = (*listSink)(nil)

func (l *listSink) Receive(resType events.ResourceType, op events.Operation, resourceId uuid.UUID, objects ...any) error {
	l.events = append(l.events, events.Event{
		ResourceType: resType,
		Operation:    op,
		ResourceId:   resourceId,
		Objects:      objects,
	})
	return nil
}
