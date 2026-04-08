package config_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"gopkg.in/yaml.v3"

	"emeland.io/modelsrv-git-sensor/internal/config"
)

var _ = Describe("config.Load", func() {
	It("loads minimal valid config and anchors relative paths", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "sensor.yaml")
		content := `
subscribers: []
watch: true
repos:
  - type: github
    repo: "https://example.com/r.git"
    branch: main
    deployKey: "k.pub"
    checkoutDir: ".work/x"
    paths: ["data"]
`
		Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())

		cfg, err := config.Load(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Watch).To(BeTrue())
		Expect(cfg.Repos).To(HaveLen(1))

		Expect(cfg.Repos[0].CheckoutDir).To(Equal(filepath.Join(dir, ".work/x")))
		Expect(cfg.Repos[0].DeployKey).To(Equal(filepath.Join(dir, "k.pub")))
		Expect(cfg.Repos[0].Paths).To(Equal([]string{"data"}))
	})

	It("trims subscribers/fields and drops empty subscribers", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "sensor.yaml")
		content := `
subscribers:
  - "  http://a/api/  "
  - ""
  - "   "
watch: false
repos:
  - type: github
    repo: "/local/repo"
    branch: "  main  "
    deployKey: "k.pub"
    checkoutDir: " .work/r "
    paths: [" p1 ", "p2"]
`
		Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())

		cfg, err := config.Load(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Subscribers).To(Equal([]string{"http://a/api/"}))
		Expect(cfg.Repos[0].Branch).To(Equal("main"))
		Expect(cfg.Repos[0].DeployKey).To(Equal(filepath.Join(dir, "k.pub")))
		Expect(cfg.Repos[0].Paths).To(Equal([]string{"p1", "p2"}))
	})

	It("defaults branch to main and repo type to github", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "sensor.yaml")
		content := `
repos:
  - repo: "https://example.com/r.git"
    deployKey: "k.pub"
    checkoutDir: ".work/x"
    paths: ["data"]
`
		Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())

		cfg, err := config.Load(path)
		Expect(err).NotTo(HaveOccurred())
		Expect(cfg.Repos[0].Branch).To(Equal("main"))
		Expect(cfg.Repos[0].Type).To(Equal("github"))
		Expect(cfg.Repos[0].CheckoutDir).To(Equal(filepath.Join(dir, ".work/x")))
	})

	DescribeTable("validation errors",
		func(y string, wantSubstr string) {
			dir := GinkgoT().TempDir()
			path := filepath.Join(dir, "c.yaml")
			Expect(os.WriteFile(path, []byte(y), 0o644)).To(Succeed())
			_, err := config.Load(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(wantSubstr))
		},
		Entry("no repos", `repos: []`, "missing repos"),
		Entry("unsupported repo type", `repos:
  - type: gitlab
    repo: "https://x"
    branch: main
    deployKey: "k.pub"
    checkoutDir: ".work/x"
    paths: ["p"]`, "unsupported type"),
		Entry("deployKey required for non-local github repo", `repos:
  - type: github
    repo: "https://x"
    branch: main
    checkoutDir: ".work/x"
    paths: ["p"]`, "deployKey required"),
		Entry("missing checkoutDir", `repos:
  - type: github
    repo: "https://x"
    branch: main
    deployKey: "k.pub"
    paths: ["p"]`, "checkoutDir"),
		Entry("missing paths", `repos:
  - type: github
    repo: "https://x"
    branch: main
    deployKey: "k.pub"
    checkoutDir: ".work/x"`, "paths"),
	)
})

var _ = Describe("config normalize/validate helpers", func() {
	It("normalizes subscriber list", func() {
		cfg := config.Config{
			Subscribers: []string{" a ", ""},
			Repos: []config.Repo{{
				Type:        "github",
				Repo:        "r",
				Branch:      "main",
				CheckoutDir: ".w",
				Paths:       []string{"p"},
			}},
		}
		config.TestOnlyNormalize(&cfg, GinkgoT().TempDir())
		Expect(cfg.Subscribers).To(Equal([]string{"a"}))
	})

	It("defaults branch to main in validate", func() {
		cfg := config.Config{
			Repos: []config.Repo{{
				Type:        "github",
				Repo:        "r",
				Branch:      "",
				DeployKey:   "k.pub",
				CheckoutDir: ".w",
				Paths:       []string{"p"},
			}},
		}
		err := config.TestOnlyValidate(cfg)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("yaml quirks", func() {
	It("errors on bad YAML", func() {
		dir := GinkgoT().TempDir()
		path := filepath.Join(dir, "bad.yaml")
		Expect(os.WriteFile(path, []byte("{not yaml"), 0o644)).To(Succeed())
		_, err := config.Load(path)
		Expect(err).To(HaveOccurred())
	})

	It("validate fails on empty unmarshaled doc", func() {
		var cfg config.Config
		Expect(yaml.Unmarshal([]byte(""), &cfg)).To(Succeed())
		Expect(config.TestOnlyValidate(cfg)).To(HaveOccurred())
	})
})
