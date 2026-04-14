package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"emeland.io/modelsrv-git-sensor/internal/fswatch"
	"emeland.io/modelsrv-git-sensor/internal/gitops"
	"emeland.io/modelsrv-git-sensor/internal/sensor"
)

// RepoType is re-exported from gitops so callers (config, main) only need to
// import this package.
type RepoType = gitops.RepoType

const (
	RepoTypeUnknown RepoType = ""
	RepoTypeGitHub           = gitops.RepoTypeGitHub
)

// ParseRepoType maps a string from config to a RepoType.
func ParseRepoType(s string) RepoType {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "github":
		return gitops.RepoTypeGitHub
	default:
		return RepoTypeUnknown
	}
}

// Config holds the full runtime configuration for one sensor process.
type Config struct {
	ListenAddr   string
	Subscribers  []string
	Repos        []RepoConfig
	PollInterval time.Duration
	WatchFS      bool
}

// RepoConfig describes a single repository the sensor should watch.
type RepoConfig struct {
	Type        RepoType
	DeployKey   string
	Repo        string
	Branch      string
	Paths       []string
	CheckoutDir string
}

// Run is the main blocking loop of the sensor. It starts the HTTP endpoint,
// checks out repositories, and reconciles YAML files on every poll/FS-event tick.
func Run(ctx context.Context, cfg Config, log *zap.SugaredLogger) error {
	if log == nil {
		log = zap.NewNop().Sugar()
	}

	s, err := sensor.New(cfg.ListenAddr, cfg.Subscribers, log)
	if err != nil {
		return err
	}

	trigger := make(chan struct{}, 1)
	notify := func() {
		select {
		case trigger <- struct{}{}:
		default:
		}
	}
	notify()

	type repoRuntime struct {
		repoCfg     RepoConfig
		state       *State
		workdir     string
		isLocal     bool
		ensureWatch func(abs string)
	}

	runtimes := make([]repoRuntime, 0, len(cfg.Repos))
	for i := range cfg.Repos {
		rc := cfg.Repos[i]
		workdir, isLocal, err := gitops.PrepareCheckout(ctx, rc.Type, rc.DeployKey, rc.Repo, rc.Branch, rc.CheckoutDir, log)
		if err != nil {
			// Don't take down the whole sensor if one repo is temporarily unavailable/misconfigured.
			log.Errorw("repo checkout failed (skipping repo)", "repoIndex", i, "repo", rc.Repo, "error", err)
			continue
		}
		log.Infow("checkout ready", "repo", rc.Repo, "dir", workdir, "branch", rc.Branch, "local", isLocal)

		var ensureWatch func(string)
		if cfg.WatchFS {
			ensureWatch = fswatch.Start(ctx, workdir, rc.Paths, notify, log)
		}

		runtimes = append(runtimes, repoRuntime{
			repoCfg:     rc,
			state:       NewState(),
			workdir:     workdir,
			isLocal:     isLocal,
			ensureWatch: ensureWatch,
		})
	}
	if len(runtimes) == 0 {
		return fmt.Errorf("no repositories available after checkout attempts")
	}

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = s.Close()
			return nil
		case <-ticker.C:
			notify()
		case <-trigger:
			for i := range runtimes {
				rt := &runtimes[i]
				rc := rt.repoCfg
				if !rt.isLocal {
					wd, _, err := gitops.PrepareCheckout(ctx, rc.Type, rc.DeployKey, rc.Repo, rc.Branch, rc.CheckoutDir, log)
					if err != nil {
						log.Errorw("git refresh failed", "repo", rc.Repo, "error", err)
						continue
					}
					rt.workdir = wd
				}

				repoRoot := rt.workdir
				for _, p := range rc.Paths {
					abs := filepath.Join(repoRoot, filepath.Clean(p))
					info, err := os.Stat(abs)
					if err != nil {
						if errors.Is(err, os.ErrNotExist) {
							log.Warnw("scan path does not exist yet (skipping)", "repo", rc.Repo, "path", abs)
						} else {
							log.Errorw("scan path stat failed", "repo", rc.Repo, "path", abs, "error", err)
						}
						continue
					}
					if rt.ensureWatch != nil {
						rt.ensureWatch(abs)
					}
					if info.IsDir() {
						if err := ReconcileDir(ctx, s, rt.state, abs, log); err != nil {
							log.Errorw("reconcile dir failed", "repo", rc.Repo, "dir", abs, "error", err)
						}
					} else {
						if err := ReconcileFile(ctx, s, rt.state, abs, log); err != nil {
							log.Errorw("reconcile file failed", "repo", rc.Repo, "file", abs, "error", err)
						}
					}
				}
			}
		}
	}
}
