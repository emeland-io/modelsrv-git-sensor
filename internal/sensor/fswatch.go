package sensor

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

const debounceDelay = 300 * time.Millisecond

// StartFSWatch registers watches on each configured scan path under workdir.
// It does not watch the whole repo root, so .git updates from fetch do not
// trigger reconcile. fsnotify does not recurse into subdirectories: a path
// listing "watchedDir" watches that directory; only direct children are reported.
// Paths that do not exist yet are skipped; call the returned function with the
// same absolute path after a successful stat (e.g. after git creates the dir).
func StartFSWatch(ctx context.Context, workdir string, relPaths []string, trigger func(), log *zap.SugaredLogger) func(abs string) {
	if trigger == nil {
		return nil
	}
	if log == nil {
		log = zap.NewNop().Sugar()
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Errorw("fswatch: create watcher failed", "error", err)
		return nil
	}

	type state struct {
		mu   sync.Mutex
		seen map[string]struct{}
	}

	st := &state{seen: make(map[string]struct{})}

	tryAdd := func(abs string) {
		abs = filepath.Clean(abs)
		st.mu.Lock()
		defer st.mu.Unlock()

		fi, statErr := os.Stat(abs)
		if statErr != nil {
			return
		}
		watchPath := abs
		if !fi.IsDir() {
			watchPath = filepath.Dir(abs)
		}
		watchPath = filepath.Clean(watchPath)
		if _, ok := st.seen[watchPath]; ok {
			return
		}
		if addErr := w.Add(watchPath); addErr != nil {
			log.Warnw("fswatch: add watch failed", "path", watchPath, "error", addErr)
			return
		}
		st.seen[watchPath] = struct{}{}
	}

	for _, p := range relPaths {
		tryAdd(filepath.Join(workdir, filepath.Clean(p)))
	}

	go func() {
		defer func() { _ = w.Close() }()

		var mu sync.Mutex
		var t *time.Timer
		schedule := func() {
			mu.Lock()
			defer mu.Unlock()
			if t != nil {
				t.Stop()
			}
			t = time.AfterFunc(debounceDelay, func() {
				select {
				case <-ctx.Done():
					return
				default:
				}
				trigger()
			})
		}

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Has(fsnotify.Create) || ev.Has(fsnotify.Write) || ev.Has(fsnotify.Rename) || ev.Has(fsnotify.Remove) {
					schedule()
				}
			case werr, ok := <-w.Errors:
				if !ok {
					return
				}
				log.Errorw("fswatch: watcher error", "error", werr)
			}
		}
	}()

	return func(abs string) { tryAdd(abs) }
}
