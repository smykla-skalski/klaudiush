package metrics_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestMetricsChecker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics Checker Suite")
}
