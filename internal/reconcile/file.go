package reconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"go.emeland.io/modelsrv/pkg/events"
	"go.emeland.io/modelsrv/pkg/filesensor"
)

// ReconcileDir scans a directory recursively for YAML files and reconciles them.
// Files that were seen in a previous pass but are no longer present trigger Delete
// events for all resources they previously contained.
func ReconcileDir(ctx context.Context, emitter Emitter, st *State, dir string, log *zap.SugaredLogger) error {
	if log == nil {
		log = zap.NewNop().Sugar()
	}

	seenFiles := make(map[string]struct{})

	walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !isYAML(path) {
			return nil
		}
		seenFiles[path] = struct{}{}
		return ReconcileFile(ctx, emitter, st, path, log)
	})
	if walkErr != nil {
		return walkErr
	}

	// Detect files that were tracked previously but no longer exist under dir.
	for _, stalePath := range st.PathsUnderDir(dir) {
		if _, still := seenFiles[stalePath]; still {
			continue
		}
		staleKeys := st.PurgePath(stalePath)
		for _, key := range staleKeys {
			rt, id, parseErr := parseKey(key)
			if parseErr != nil {
				log.Warnw("could not parse stale key (skipping delete)", "key", key, "error", parseErr)
				continue
			}
			ev := events.Event{
				ResourceType: rt,
				Operation:    events.DeleteOperation,
				ResourceId:   id,
			}
			if err := emitter.Emit(ev); err != nil {
				log.Errorw("delete emit failed (removed file)", "file", stalePath, "kind", rt.String(), "id", id.String(), "error", err)
			} else {
				log.Infow("event emitted (removed file)", "file", stalePath, "kind", rt.String(), "id", id.String(), "operation", ev.Operation.WireOperation())
			}
		}
	}

	return nil
}

// ReconcileFile reads a single YAML file, decodes its documents, and emits
// Create, Update, or Delete events as appropriate compared to the previous pass.
func ReconcileFile(ctx context.Context, emitter Emitter, st *State, path string, log *zap.SugaredLogger) error {
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

	// Track which keys are present in this reconcile pass so we can
	// detect documents that were removed from the file.
	oldKeys := st.KeysForPath(path)
	seenKeys := make(map[string]struct{}, len(docs))

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

		seenKeys[key] = struct{}{}

		prev, existed := st.Get(key)
		if existed && prev == hash {
			continue
		}

		ev, err := buildEvent(doc)
		if err != nil {
			log.Warnw("build event failed (skipping)", "file", path, "kind", rt.String(), "error", err)
			continue
		}
		if existed {
			ev.Operation = events.UpdateOperation
		} else {
			ev.Operation = events.CreateOperation
		}

		if err := emitter.Emit(ev); err != nil {
			log.Errorw("emit failed", "file", path, "kind", rt.String(), "id", id.String(), "error", err)
			continue
		}
		log.Infow("event emitted", "file", path, "kind", rt.String(), "id", id.String(), "operation", ev.Operation.WireOperation())

		st.Set(key, hash)
	}

	// Emit Delete for any resource that was previously in this file but is no longer.
	for _, key := range oldKeys {
		if _, still := seenKeys[key]; still {
			continue
		}
		rt, id, parseErr := parseKey(key)
		if parseErr != nil {
			log.Warnw("could not parse stale key (skipping delete)", "key", key, "error", parseErr)
			st.DeleteKey(key)
			continue
		}
		ev := events.Event{
			ResourceType: rt,
			Operation:    events.DeleteOperation,
			ResourceId:   id,
		}
		if err := emitter.Emit(ev); err != nil {
			log.Errorw("delete emit failed", "file", path, "kind", rt.String(), "id", id.String(), "error", err)
		} else {
			log.Infow("event emitted", "file", path, "kind", rt.String(), "id", id.String(), "operation", ev.Operation.WireOperation())
		}
		st.DeleteKey(key)
	}

	// Replace the path's key set with exactly what we saw this pass.
	st.SetKeysForPath(path, seenKeys)
	return nil
}

func isYAML(path string) bool {
	p := strings.ToLower(path)
	return strings.HasSuffix(p, ".yaml") || strings.HasSuffix(p, ".yml")
}
