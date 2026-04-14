package sensor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

type RepoType string

const (
	RepoTypeGitHub RepoType = "github"
)

// PrepareCheckout returns a directory containing the repo at the desired branch/ref.
// If repo is a local path, it is used directly (no clone); a best-effort fetch/checkout is attempted.
func PrepareCheckout(ctx context.Context, repoType RepoType, deployKeyPubPath string, repo string, branch string, checkoutDir string, log *zap.SugaredLogger) (dir string, isLocal bool, err error) {
	if log == nil {
		log = zap.NewNop().Sugar()
	}

	repo = strings.TrimSpace(repo)
	if repo == "" {
		return "", false, fmt.Errorf("empty repo")
	}

	if fi, statErr := os.Stat(repo); statErr == nil && fi.IsDir() {
		dir = repo
		var absErr error
		if dir, absErr = filepath.Abs(dir); absErr != nil {
			return "", true, absErr
		}
		_ = git(ctx, dir, log, nil, "fetch", "--all", "--prune")
		_ = git(ctx, dir, log, nil, "checkout", branch)
		_ = git(ctx, dir, log, nil, "pull", "--ff-only")
		return dir, true, nil
	}

	dir = checkoutDir
	if strings.TrimSpace(dir) == "" {
		return "", false, fmt.Errorf("empty checkoutDir")
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return "", false, err
	}

	var env []string
	if repoType == RepoTypeGitHub {
		deployKeyPubPath = strings.TrimSpace(deployKeyPubPath)
		if deployKeyPubPath == "" {
			return "", false, fmt.Errorf("deployKey is required for github repo checkouts")
		}
		priv := inferPrivateKeyPathFromPub(deployKeyPubPath)
		var cleanup func()
		env, cleanup, err = gitSSHEnv(priv)
		if err != nil {
			return "", false, err
		}
		defer cleanup()

		// For deploy keys we prefer cloning over SSH. If config provides an HTTPS GitHub URL,
		// convert it to the equivalent SSH URL to match Flux-style deploy key usage.
		if strings.HasPrefix(strings.ToLower(repo), "http://") || strings.HasPrefix(strings.ToLower(repo), "https://") {
			if sshURL, convOk := githubHTTPSURLToSSH(repo); convOk {
				repo = sshURL
			}
		}
	}

	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		// Fresh clone.
		if err := run(ctx, "", log, env, "git", "clone", repo, dir); err != nil {
			return "", false, err
		}
	}

	if err := git(ctx, dir, log, env, "fetch", "--all", "--prune"); err != nil {
		return "", false, err
	}

	// Support both branches and refs.
	if err := git(ctx, dir, log, env, "checkout", branch); err != nil {
		// Try origin/<branch> as fallback.
		if err2 := git(ctx, dir, log, env, "checkout", "-B", branch, "origin/"+branch); err2 != nil {
			// Some repos don't have origin/<branch> (e.g. default branch differs).
			// Fall back to origin/HEAD (remote default) so the sensor can still run.
			if err3 := git(ctx, dir, log, env, "checkout", "-B", branch, "origin/HEAD"); err3 != nil {
				return "", false, err
			}
		}
	}
	_ = git(ctx, dir, log, env, "reset", "--hard", "origin/"+branch)

	if dir, err = filepath.Abs(dir); err != nil {
		return "", false, err
	}
	return dir, false, nil
}

func git(ctx context.Context, dir string, log *zap.SugaredLogger, env []string, args ...string) error {
	return run(ctx, dir, log, env, "git", args...)
}

func run(ctx context.Context, dir string, log *zap.SugaredLogger, env []string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if err != nil {
		if log != nil {
			log.Errorw("command failed", "cmd", name, "args", args, "dir", dir, "output", strings.TrimSpace(buf.String()), "error", err)
		}
		return fmt.Errorf("%s %v failed: %w", name, args, err)
	}
	return nil
}

func inferPrivateKeyPathFromPub(pubPath string) string {
	pubPath = strings.TrimSpace(pubPath)
	if strings.HasSuffix(pubPath, ".pub") {
		return strings.TrimSuffix(pubPath, ".pub")
	}
	return pubPath
}

func githubHTTPSURLToSSH(repo string) (string, bool) {
	// Accept forms:
	// - https://github.com/OWNER/REPO
	// - https://github.com/OWNER/REPO.git
	// And return: git@github.com:OWNER/REPO.git
	r := strings.TrimSpace(repo)
	rl := strings.ToLower(r)
	if !strings.HasPrefix(rl, "https://github.com/") && !strings.HasPrefix(rl, "http://github.com/") {
		return "", false
	}
	// Strip scheme + host.
	parts := strings.SplitN(r, "github.com/", 2)
	if len(parts) != 2 {
		return "", false
	}
	path := strings.TrimPrefix(parts[1], "/")
	path = strings.TrimSuffix(path, "/")
	if path == "" {
		return "", false
	}
	if !strings.HasSuffix(path, ".git") {
		path += ".git"
	}
	// Basic sanity: require OWNER/REPO.
	if !strings.Contains(path, "/") {
		return "", false
	}
	return "git@github.com:" + path, true
}

// Test-only helpers.
func TestOnlyGithubHTTPSURLToSSH(repo string) (string, bool) { return githubHTTPSURLToSSH(repo) }
func TestOnlyInferPrivateKeyPathFromPub(pubPath string) string {
	return inferPrivateKeyPathFromPub(pubPath)
}

func gitSSHEnv(privateKeyPath string) (env []string, cleanup func(), err error) {
	privateKeyPath = strings.TrimSpace(privateKeyPath)
	if privateKeyPath == "" {
		return nil, nil, fmt.Errorf("empty private key path")
	}
	if _, err := os.Stat(privateKeyPath); err != nil {
		return nil, nil, fmt.Errorf("ssh key not found at %q: %w", privateKeyPath, err)
	}
	khDir, err := os.MkdirTemp("", "modelsrv-git-sensor-knownhosts-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup = func() { _ = os.RemoveAll(khDir) }
	knownHosts := filepath.Join(khDir, "known_hosts")
	_ = os.WriteFile(knownHosts, []byte{}, 0o600)

	sshCmd := "ssh -i " + shellQuote(privateKeyPath) +
		" -o IdentitiesOnly=yes" +
		" -o StrictHostKeyChecking=accept-new" +
		" -o UserKnownHostsFile=" + shellQuote(knownHosts)

	env = []string{
		"GIT_TERMINAL_PROMPT=0",
		"GIT_SSH_COMMAND=" + sshCmd,
	}
	return env, cleanup, nil
}

func shellQuote(s string) string {
	// Minimal POSIX shell quoting for paths (avoid spaces breaking ssh command).
	// This is sufficient for file paths; it doesn't aim to be a full shell escaper.
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
