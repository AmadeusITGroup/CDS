# define a variable if it is not already defined
GOLANGCI_LINT_VERSION := latest
GOPATH := $(shell go env GOPATH)
GO_MOD_VERSION := $(shell grep '^go ' go.mod | awk '{print $$2}')
PATH := $(PATH):$(GOPATH)/bin
ECHO = echo -e

# check protoc is installed
$(if $(shell which protoc),,$(error protoc is not installed. Please install it))

ifeq ($(OS),Windows_NT)
	HOME_DIR=$(shell cygpath -u $(HOME))
else
	HOME_DIR=$(HOME)
endif
TEST_FOLDER=test/
ifeq ($(OS),Windows_NT)
    ECHO_BEFORE=
	ECHO_BEFORE2=
	ECHO_AFTER=
else
	ECHO_BEFORE=\033[1;93m
	ECHO_BEFORE2=\033[1;34m
	ECHO_AFTER=\033[0m
endif
CDS_CONFIG_PATH=${HOME_DIR}/cdstmp

# check that all required variables are set
# vars := GOLANGCI_LINT_VERSION GOPATH GOPRIVATE GOPROXY CDS_CONFIG_PATH
vars := GOLANGCI_LINT_VERSION GOPATH CDS_CONFIG_PATH
$(foreach var, $(vars), $(if $(value $(var)), $(info $(var)=$(value $(var))), $(error $(var) is not set)))

.PHONY: help \
	install \
	lint \
	lint-weak \
	run-api-agent \
	run-client \
	run-metrics-analyzer \
	build-pb \
	build-api-agent \
	build-client \
	build-metrics-analyzer \
	test \
	coverage \
	go-tidy \
	install-golangci-lint \
	init \
	gencert \
	scaffold

help:
	@$(ECHO) "$(ECHO_BEFORE)CDS (Continuous Delivery Service) Makefile$(ECHO_AFTER)"
	@$(ECHO) "A build, test, and run system for the CDS Go application, providing both a"
	@$(ECHO) "gRPC API agent and a CLI client."
	@$(ECHO) ""
	@$(ECHO) "$(ECHO_BEFORE2)Usage$(ECHO_AFTER)"
	@$(ECHO) "  make [target]"
	@$(ECHO) ""
	@$(ECHO) "$(ECHO_BEFORE2)High-level targets$(ECHO_AFTER)"
	@$(ECHO) "  install      Full pipeline: build, test, coverage"
	@$(ECHO) "  build        Build binaries (lint + protobuf + binaries)"
	@$(ECHO) "  ci-build     CI-optimized build (no lint)"
	@$(ECHO) "  scaffold     Full pipeline: build, test, coverage (complete)"
	@$(ECHO) ""
	@$(ECHO) "$(ECHO_BEFORE2)Binary targets$(ECHO_AFTER)"
	@$(ECHO) "  build-api-agent    Compile the gRPC API agent server"
	@$(ECHO) "  build-client       Compile the CDS CLI client"
	@$(ECHO) "  build-pb           Generate Go code from .proto files"
	@$(ECHO) ""
	@$(ECHO) "$(ECHO_BEFORE2)Quality targets$(ECHO_AFTER)"
	@$(ECHO) "  lint         Run golangci-lint on all packages"
	@$(ECHO) "  lint-weak    Run lint excluding unused checks"
	@$(ECHO) "  test         Run all Go tests with verbose output"
	@$(ECHO) "  coverage     Generate HTML coverage report"
	@$(ECHO) "  gen-coverage Run tests with coverage profiling"
	@$(ECHO) ""
	@$(ECHO) "$(ECHO_BEFORE2)Run targets$(ECHO_AFTER)"
	@$(ECHO) "  run-api-agent       Run the API agent server"
	@$(ECHO) "  run-client          Run the CDS CLI"
	@$(ECHO) "  run-metrics-analyzer Run the metrics analyzer"
	@$(ECHO) ""
	@$(ECHO) "$(ECHO_BEFORE2)Utility targets$(ECHO_AFTER)"
	@$(ECHO) "  go-tidy              Run go mod tidy"
	@$(ECHO) "  init                 Create required directories"
	@$(ECHO) "  gencert              Generate TLS certificates for testing"
	@$(ECHO) "  install-golangci-lint Install golangci-lint locally"
	@$(ECHO) "  help                 Show this help message"

install: \
	build \
	test \
	coverage

build: \
	init \
	gencert \
	build-pb \
	lint \
	build-api-agent \
	build-client

ci-build: \
	init \
	gencert \
	build-pb \
	build-api-agent \
	build-client \

scaffold: \
	init \
	gencert \
	build-pb \
	lint \
	build-api-agent \
	build-client \
	test \
	gen-coverage \
	coverage

init:
	@$(ECHO) "$(ECHO_BEFORE)Creating certs directory$(ECHO_AFTER)"
	mkdir -p $(TEST_FOLDER) $(CDS_CONFIG_PATH)/.xcds/certs

gencert: init
	CDS_CONFIG_PATH=${HOME_DIR}/cdstmp
	@$(ECHO) "$(ECHO_BEFORE)Generating certificates$(ECHO_AFTER)"
	go install github.com/cloudflare/cfssl/cmd/cfssl@latest
	go install github.com/cloudflare/cfssl/cmd/cfssljson@latest
	cfssl gencert \
		-initca $(TEST_FOLDER)ca-csr.json | cfssljson -bare ca
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=$(TEST_FOLDER)ca-config.json \
		-profile=server \
		$(TEST_FOLDER)server-csr.json | cfssljson -bare agent-srv
	cfssl gencert \
		-ca=ca.pem \
		-ca-key=ca-key.pem \
		-config=$(TEST_FOLDER)ca-config.json \
		-profile=client \
		$(TEST_FOLDER)client-csr.json | cfssljson -bare client
	mv *.pem *.csr $(CDS_CONFIG_PATH)/.xcds/certs

go-tidy: build-pb
	@$(ECHO) "$(ECHO_BEFORE)Executing go mod tidy$(ECHO_AFTER)"
	go mod tidy

lint: install-golangci-lint
	@$(ECHO) "$(ECHO_BEFORE)Executing lint$(ECHO_AFTER)"
	golangci-lint run ./...

lint-weak: install-golangci-lint
	@$(ECHO) "$(ECHO_BEFORE)Executing weak lint$(ECHO_AFTER)"
	golangci-lint run ./... --exclude 'is unused'

install-golangci-lint:
	@$(ECHO) "$(ECHO_BEFORE)Executing install-golangci-lint$(ECHO_AFTER)"
	which golangci-lint || GOTOOLCHAIN=go$(GO_MOD_VERSION) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

run-api-agent: go-tidy
	@$(ECHO) "$(ECHO_BEFORE2)Running cds api server$(ECHO_AFTER)"
	go run ./cmd/api-agent/cds-api-agent.go start

run-client: go-tidy
	@$(ECHO) "$(ECHO_BEFORE2)Running cds CLI$(ECHO_AFTER)"
	go run ./cmd/client/cds.go

run-metrics-analyzer: go-tidy
	@$(ECHO) "$(ECHO_BEFORE2)Running metrics analyzer$(ECHO_AFTER)"
	go run ./cmd/metrics-analyzer/analyzer.go

build-api-agent: go-tidy build-pb
	@$(ECHO) "$(ECHO_BEFORE2)Building cds api server$(ECHO_AFTER)"
	go build -o cds-api-agent ./cmd/api-agent/cds-api-agent.go

build-client: go-tidy build-pb
	@$(ECHO) "$(ECHO_BEFORE2)Building cds CLI$(ECHO_AFTER)"
	go build -o cds ./cmd/client/cds.go

build-pb:
	@$(ECHO) "$(ECHO_BEFORE2)Building protobuf$(ECHO_AFTER)"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	mkdir -p internal/api/v1/cdspb
	rm -f internal/api/v1/cdspb/*.pb.go
	protoc --proto_path=. \
		--go_out=internal/api/v1 --go_opt=module=github.com/amadeusitgroup/cds \
		--go-grpc_out=internal/api/v1 --go-grpc_opt=module=github.com/amadeusitgroup/cds \
		$$(find internal/api/v1 -name '*.proto' | LC_ALL=C sort)
test:
	@$(ECHO) "$(ECHO_BEFORE2)Executing tests$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go test ./... -v

gen-coverage:
	@$(ECHO) "$(ECHO_BEFORE2)Executing coverage$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go test $$(go list -f '{{if (or .TestGoFiles .XTestGoFiles)}}{{.ImportPath}}{{end}}' ./...) -coverprofile=coverage.out

coverage: gen-coverage
	@$(ECHO) "$(ECHO_BEFORE2)Generating coverage report$(ECHO_AFTER)"
	go tool cover -html=coverage.out

# Include delivery targets
include makefile.delivery
