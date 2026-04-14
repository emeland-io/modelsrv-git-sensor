package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Subscribers []string `yaml:"subscribers"`
	Watch       bool     `yaml:"watch"`
	Repos       []Repo   `yaml:"repos"`
}

type Repo struct {
	Type        string   `yaml:"type"`
	DeployKey   string   `yaml:"deployKey"`
	Repo        string   `yaml:"repo"`
	Branch      string   `yaml:"branch"`
	CheckoutDir string   `yaml:"checkoutDir"`
	Paths       []string `yaml:"paths"`
}

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	baseDir := filepath.Dir(path)
	if abs, err := filepath.Abs(baseDir); err == nil {
		baseDir = abs
	}
	normalize(&cfg, baseDir)
	if err := validate(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func normalize(cfg *Config, baseDir string) {
	subs := make([]string, 0, len(cfg.Subscribers))
	for _, s := range cfg.Subscribers {
		s = strings.TrimSpace(s)
		if s != "" {
			subs = append(subs, s)
		}
	}
	cfg.Subscribers = subs

	repos := make([]Repo, 0, len(cfg.Repos))
	for _, r := range cfg.Repos {
		r.Type = strings.ToLower(strings.TrimSpace(r.Type))
		r.DeployKey = strings.TrimSpace(r.DeployKey)
		r.Repo = strings.TrimSpace(r.Repo)
		r.Branch = strings.TrimSpace(r.Branch)
		r.CheckoutDir = strings.TrimSpace(r.CheckoutDir)
		// Prevent drift: resolve relative paths from config directory, not process cwd.
		if r.CheckoutDir != "" && !filepath.IsAbs(r.CheckoutDir) {
			r.CheckoutDir = filepath.Join(baseDir, r.CheckoutDir)
		}
		if r.DeployKey != "" && !filepath.IsAbs(r.DeployKey) {
			r.DeployKey = filepath.Join(baseDir, r.DeployKey)
		}
		// If repo is a local path and relative, anchor it too.
		if r.Repo != "" && !strings.Contains(r.Repo, "://") && !strings.Contains(r.Repo, "@") && !filepath.IsAbs(r.Repo) {
			r.Repo = filepath.Join(baseDir, r.Repo)
		}
		paths := make([]string, 0, len(r.Paths))
		for _, p := range r.Paths {
			p = strings.TrimSpace(p)
			if p != "" {
				paths = append(paths, p)
			}
		}
		r.Paths = paths
		repos = append(repos, r)
	}
	cfg.Repos = repos
}

func validate(cfg Config) error {
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("config: missing repos")
	}
	for i := range cfg.Repos {
		r := cfg.Repos[i]
		if r.Type == "" {
			r.Type = "github"
			cfg.Repos[i].Type = "github"
		}
		if r.Type != "github" {
			return fmt.Errorf("config: repos[%d]: unsupported type %q (only \"github\" supported)", i, r.Type)
		}
		if r.Repo == "" {
			return fmt.Errorf("config: repos[%d]: missing repo", i)
		}
		// Flux-style default: use deploy keys for non-local checkouts.
		// Local repos can omit deployKey.
		if _, statErr := os.Stat(r.Repo); statErr != nil {
			if strings.TrimSpace(r.DeployKey) == "" {
				return fmt.Errorf("config: repos[%d]: deployKey required for non-local github repo", i)
			}
		}
		if r.Branch == "" {
			r.Branch = "main"
			cfg.Repos[i].Branch = "main"
		}
		if len(r.Paths) == 0 {
			return fmt.Errorf("config: repos[%d]: missing paths", i)
		}
		if r.CheckoutDir == "" {
			return fmt.Errorf("config: repos[%d]: missing checkoutDir", i)
		}
	}
	return nil
}

