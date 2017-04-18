# Disable make's implicit rules, which are not useful for golang, and
# slow down the build considerably.

VPATH=bin:plugins/l4lb:pkg/spartan

# Default go OS to linux
export GOOS?=linux

# Set $GOPATH to a local directory so that we are not influenced by
# the hierarchical structure of an existing $GOPATH directory.
export GOPATH=$(shell pwd)/gopath

ifeq ($(VERBOSE),1)
	TEST_VERBOSE=-ginkgo.v
endif

#dcos-l4lb
L4LB=github.com/dcos/dcos-cni/plugins/l4lb
L4LB_SRC=$(wildcard plugins/l4lb/*.go) $(wildcard pkg/spartan/*.go)
L4LB_TEST_SRC=$(wildcard plugins/l4lbl/*_tests.go)

PLUGINS=dcos-l4lb
TESTS=dcos-l4lb-test

.PHONY: all plugin
default: all

.PHONY: clean
clean:
	rm -rf vendor bin gopath

gopath:
	mkdir -p gopath/src/github.com/dcos
	ln -s `pwd` gopath/src/github.com/dcos/dcos-cni

# To update upstream dependencies, delete the glide.lock file first.
# Use this to populate the vendor directory after checking out the
# repository.
vendor: glide.yaml
	echo $(GOPATH)
	glide install -strip-vendor

dcos-l4lb:$(L4LB_SRC)
	echo "GOPATH:" $(GOPATH)
	mkdir -p `pwd`/bin
	go build -v -o `pwd`/bin/$@ $(L4LB)

plugin: gopath vendor $(PLUGINS)

dcos-l4lb-test:$(L4LB_TEST_SRC)
	echo "GOPATH:" $(GOPATH)
	go test $(L4LB) $(TEST_VERBOSE)

tests: $(TESTS)

all: plugin

