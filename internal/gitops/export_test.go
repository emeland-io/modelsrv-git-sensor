package gitops

// Export* symbols are test hooks (this file is not compiled into non-test builds).
// Names avoid the "Test*" prefix so the Go test harness does not treat them as tests.

func ExportGithubHTTPSURLToSSH(repo string) (string, bool) { return githubHTTPSURLToSSH(repo) }

func ExportInferPrivateKeyPathFromPub(pubPath string) string {
	return inferPrivateKeyPathFromPub(pubPath)
}
