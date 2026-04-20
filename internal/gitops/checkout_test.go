package gitops_test

import (
	"context"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"emeland.io/modelsrv-git-sensor/internal/gitops"
)

var _ = Describe("git checkout helpers", func() {
	It("returns absolute path for local repo checkouts", func() {
		tmp := GinkgoT().TempDir()
		want, err := filepath.Abs(tmp)
		Expect(err).NotTo(HaveOccurred())

		log := zap.NewNop().Sugar()
		dir, isLocal, err := gitops.PrepareCheckout(context.Background(), gitops.RepoTypeGitHub, "", tmp, "main", "ignored-for-local", log)
		Expect(err).NotTo(HaveOccurred())
		Expect(isLocal).To(BeTrue())
		Expect(dir).To(Equal(want))
	})

	It("errors on empty checkoutDir for URL repo", func() {
		_, _, err := gitops.PrepareCheckout(context.Background(), gitops.RepoTypeGitHub, "nope.pub", "https://example.com/none.git", "main", "", zap.NewNop().Sugar())
		Expect(err).To(HaveOccurred())
	})

	It("allows SSH-style URLs (deploy key flow)", func() {
		// This will fail because the key doesn't exist, but it must not fail due to URL shape.
		_, _, err := gitops.PrepareCheckout(context.Background(), gitops.RepoTypeGitHub, "nope.pub", "git@github.com:org/repo.git", "main", ".work/checkout/x", zap.NewNop().Sugar())
		Expect(err).To(HaveOccurred())
	})

	It("converts GitHub HTTPS URLs to SSH", func() {
		got, ok := gitops.ExportGithubHTTPSURLToSSH("https://github.com/ORG/REPO")
		Expect(ok).To(BeTrue())
		Expect(got).To(Equal("git@github.com:ORG/REPO.git"))
	})

	It("infers private key from .pub path", func() {
		Expect(gitops.ExportInferPrivateKeyPathFromPub("xhela_deploy_key.pub")).To(Equal("xhela_deploy_key"))
	})
})
