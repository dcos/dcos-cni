package main

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestL4lb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "L4lb Suite")
}
