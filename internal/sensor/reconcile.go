package sensor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/events"
	"go.emeland.io/modelsrv/pkg/filesensor"
	"go.emeland.io/modelsrv/pkg/model"
)

// ReconcileDir scans a directory recursively for YAML files and reconciles them.
func ReconcileDir(ctx context.Context, srv *sensorServer, st *MemState, dir string, log *zap.SugaredLogger) error {
	if log == nil {
		log = zap.NewNop().Sugar()
	}
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isYAML(path) {
			return nil
		}
		return ReconcileFile(ctx, srv, st, path, log)
	})
}

func ReconcileFile(ctx context.Context, srv *sensorServer, st *MemState, path string, log *zap.SugaredLogger) error {
	if log == nil {
		log = zap.NewNop().Sugar()
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	docs, err := decodeDocuments(b)
	if err != nil {
		// Not a valid emeland YAML file => ignore quietly.
		return nil
	}
	for i := range docs {
		doc := docs[i]
		if !filesensor.ValidVersion(doc.Version) {
			continue
		}
		rt := doc.Kind.ResourceType()
		if !supported(rt) {
			log.Warnw("unsupported kind (skipping)", "file", path, "kind", rt.String())
			continue
		}

		id, err := extractID(rt, doc.Spec)
		if err != nil {
			log.Warnw("invalid document id (skipping)", "file", path, "kind", rt.String(), "error", err)
			continue
		}

		key := fmt.Sprintf("%s/%s", rt.String(), id.String())
		hash, err := StableHash(map[string]any{
			"version": doc.Version,
			"kind":    rt.String(),
			"spec":    doc.Spec,
		})
		if err != nil {
			return err
		}
		if prev, ok := st.Get(key); ok && prev == hash {
			continue
		}

		ev, err := buildCreateEvent(doc)
		if err != nil {
			log.Warnw("build event failed (skipping)", "file", path, "kind", rt.String(), "error", err)
			continue
		}
		// Always Create; receiver can up-convert.
		ev.Operation = events.CreateOperation

		if err := srv.Emit(ev); err != nil {
			log.Errorw("emit failed", "file", path, "kind", rt.String(), "id", id.String(), "error", err)
			continue
		}
		log.Infow("event emitted", "file", path, "kind", rt.String(), "id", id.String(), "operation", ev.Operation.WireOperation())

		st.Set(key, hash)
	}
	return nil
}

func isYAML(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")
}

func supported(rt events.ResourceType) bool {
	switch rt {
	case events.SystemResource,
		events.SystemInstanceResource,
		events.APIResource,
		events.APIInstanceResource,
		events.ComponentResource,
		events.ComponentInstanceResource:
		return true
	default:
		return false
	}
}

func extractID(rt events.ResourceType, spec map[string]any) (uuid.UUID, error) {
	switch rt {
	case events.SystemResource:
		return uuidFromSpec(spec, "systemId")
	case events.SystemInstanceResource:
		// spec uses systemInstanceId in YAML? modelsrv file sensor accepts systemInstanceId.
		return uuidFromFirst(spec, "systemInstanceId", "instanceId")
	case events.APIResource:
		return uuidFromSpec(spec, "apiId")
	case events.APIInstanceResource:
		return uuidFromFirst(spec, "apiInstanceId", "instanceId")
	case events.ComponentResource:
		return uuidFromSpec(spec, "componentId")
	case events.ComponentInstanceResource:
		return uuidFromFirst(spec, "componentInstanceId", "instanceId")
	default:
		return uuid.Nil, fmt.Errorf("unsupported resource type %s", rt.String())
	}
}

// TestOnlyExtractID is a test helper for validating ID extraction rules.
// It is not considered part of the public API of the sensor.
func TestOnlyExtractID(rt events.ResourceType, spec map[string]any) (uuid.UUID, error) {
	return extractID(rt, spec)
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

// buildCreateEvent applies a decoded YAML document into a temporary in-memory model and returns the resulting event.
func buildCreateEvent(doc filesensor.Document) (events.Event, error) {
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

