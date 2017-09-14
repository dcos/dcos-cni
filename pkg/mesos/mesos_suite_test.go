package mesos_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMesos(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mesos Suite")
}
