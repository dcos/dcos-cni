package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"os"

	"testing"
)

var osPath string

func TestL4lb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "L4lb Suite")
}

var _ = BeforeSuite(func() {
	osPath = os.Getenv("PATH")
	cniPath, present := os.LookupEnv("CNI_PLUGIN_PATH")
	Expect(present).To(Equal(true), "We cannot run these tests without a `CNI_PLUGIN_PATH` set")

	os.Setenv("PATH", cniPath+":"+osPath)
})

var _ = AfterSuite(func() {
	os.Setenv("PATH", osPath)
})
