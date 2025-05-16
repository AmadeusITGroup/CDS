package ar_test

import (
	"testing"

	"github.com/jarcoal/httpmock"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

var _ = ginkgo.BeforeSuite(func() {
	// block all HTTP requests
	httpmock.Activate()
})

var _ = ginkgo.BeforeEach(func() {
	// remove any mocks
	httpmock.Reset()
})

var _ = ginkgo.AfterSuite(func() {
	httpmock.DeactivateAndReset()
})

func TestNet(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "Artifactory Suite")
}
