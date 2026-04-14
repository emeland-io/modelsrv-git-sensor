package fswatch_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/zap"

	"emeland.io/modelsrv-git-sensor/internal/fswatch"
)

var _ = Describe("fswatch", func() {
	It("returns nil ensure func when trigger is nil", func() {
		got := fswatch.Start(context.Background(), GinkgoT().TempDir(), []string{"."}, nil, zap.NewNop().Sugar())
		Expect(got).To(BeNil())
	})
})
