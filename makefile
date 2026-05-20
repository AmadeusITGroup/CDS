# define variables if they are not already defined
GOLANGCI_LINT_VERSION ?= v2.6.2
CFSSL_VERSION ?= v1.6.5
PROTOC_GEN_GO_VERSION ?= v1.36.10
PROTOC_GEN_GO_GRPC_VERSION ?= v1.5.1
GOPATH := $(shell go env GOPATH)
PATH := $(PATH):$(GOPATH)/bin

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
CDS_CONFIG_PATH ?= ${HOME_DIR}/cdstmp

# check that all required variables are set
# vars := GOLANGCI_LINT_VERSION GOPATH GOPRIVATE GOPROXY CDS_CONFIG_PATH
vars := GOLANGCI_LINT_VERSION GOPATH CDS_CONFIG_PATH
$(foreach var, $(vars), $(if $($(var)), $(info $(var)=$($(var))), $(error $(var) is not set)))

.PHONY: install \
	help \
	check \
	fmt \
	setup-tools \
	lint \
	lint-weak \
	run-api-agent \
	run-client \
	run-metrics-analyzer \
	build-pb \
	build-api-agent \
	build-api-agent-fast \
	build-client \
	build-client-fast \
	build-fast \
	build-metrics-analyzer \
	test \
	test-fast \
	coverage \
	go-tidy \
	install-cfssl \
	install-golangci-lint \
	install-protobuf-tools \
	init \
	gencert \
	scaffold

help:
	@echo "CDS developer workflow targets:"
	@echo "  make setup-tools       Install pinned Go-based build tools"
	@echo "  make build-fast        Build CLI and agent without generation, tidy, or lint"
	@echo "  make test-fast         Run the most common fast package tests"
	@echo "  make check             Run the local pre-PR validation flow"
	@echo "  make build             Generate certs, lint, generate protobuf, tidy, and build binaries"
	@echo "  make test              Run the full Go test suite"
	@echo "  make lint              Run golangci-lint"
	@echo "  make build-pb          Regenerate protobuf Go files"
	@echo "  make coverage          Generate and open an HTML coverage report"
	@echo "  make run-client        Run the CDS CLI from source"
	@echo "  make run-api-agent     Run the CDS API agent from source"

setup-tools: \
	install-cfssl \
	install-golangci-lint \
	install-protobuf-tools

check: \
	init \
	gencert \
	fmt \
	build-pb \
	go-tidy \
	lint \
	test \
	build-fast

install: \
	build \
	test \
	coverage

build: \
	init \
	gencert \
	lint \
	build-pb \
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
	lint \
	build-pb \
	build-api-agent \
	build-client \
	test \
	gen-coverage \
	coverage

init:
	@echo "$(ECHO_BEFORE)Creating certs directory$(ECHO_AFTER)"
	mkdir -p $(TEST_FOLDER) $(CDS_CONFIG_PATH)/.xcds/certs $(CDS_CONFIG_PATH)/.xcds-agent/certs

gencert: init install-cfssl
	@echo "$(ECHO_BEFORE)Generating certificates$(ECHO_AFTER)"
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
	rm -f $(CDS_CONFIG_PATH)/.xcds/certs/*.pem $(CDS_CONFIG_PATH)/.xcds/certs/*.csr $(CDS_CONFIG_PATH)/.xcds-agent/certs/*.pem $(CDS_CONFIG_PATH)/.xcds-agent/certs/*.csr
	cp ca.pem client.pem client-key.pem $(CDS_CONFIG_PATH)/.xcds/certs
	cp ca.pem agent-srv.pem agent-srv-key.pem $(CDS_CONFIG_PATH)/.xcds-agent/certs
	rm -rf $(CDS_CONFIG_PATH)/.xcds/certsjson $(CDS_CONFIG_PATH)/.xcds-agent/certsjson
	rm -f *.pem *.csr

go-tidy: build-pb
	@echo "$(ECHO_BEFORE)Executing go mod tidy$(ECHO_AFTER)"
	go mod tidy

fmt:
	@echo "$(ECHO_BEFORE)Formatting Go files$(ECHO_AFTER)"
	find . -name '*.go' -not -path './internal/api/v1/cdspb/*' -print0 | xargs -0 gofmt -w

lint: install-golangci-lint
	@echo "$(ECHO_BEFORE)Executing lint$(ECHO_AFTER)"
	golangci-lint run ./...

lint-weak: install-golangci-lint
	@echo "$(ECHO_BEFORE)Executing weak lint$(ECHO_AFTER)"
	golangci-lint run ./... --exclude 'is unused'

install-golangci-lint:
	@echo "$(ECHO_BEFORE)Executing install-golangci-lint$(ECHO_AFTER)"
	which golangci-lint || curl -sSfL https://golangci-lint.run/install.sh | sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)

install-cfssl:
	@echo "$(ECHO_BEFORE)Executing install-cfssl$(ECHO_AFTER)"
	go install github.com/cloudflare/cfssl/cmd/cfssl@$(CFSSL_VERSION)
	go install github.com/cloudflare/cfssl/cmd/cfssljson@$(CFSSL_VERSION)

install-protobuf-tools:
	@echo "$(ECHO_BEFORE)Executing install-protobuf-tools$(ECHO_AFTER)"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION)
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@$(PROTOC_GEN_GO_GRPC_VERSION)

run-api-agent: go-tidy
	@echo "$(ECHO_BEFORE2)Running cds api server$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go run ./cmd/api-agent/cds-api-agent.go start

run-client: go-tidy
	@echo "$(ECHO_BEFORE2)Running cds CLI$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go run ./cmd/client/cds.go

run-metrics-analyzer: go-tidy
	@echo "$(ECHO_BEFORE2)Running metrics analyzer$(ECHO_AFTER)"
	go run ./cmd/metrics-analyzer/analyzer.go

build-api-agent: go-tidy build-pb
	@echo "$(ECHO_BEFORE2)Building cds api server$(ECHO_AFTER)"
	go build -o cds-api-agent ./cmd/api-agent/cds-api-agent.go

build-api-agent-fast:
	@echo "$(ECHO_BEFORE2)Building cds api server without generation$(ECHO_AFTER)"
	go build -o cds-api-agent ./cmd/api-agent/cds-api-agent.go

build-client: go-tidy build-pb
	@echo "$(ECHO_BEFORE2)Building cds CLI$(ECHO_AFTER)"
	go build -o cds ./cmd/client/cds.go

build-client-fast:
	@echo "$(ECHO_BEFORE2)Building cds CLI without generation$(ECHO_AFTER)"
	go build -o cds ./cmd/client/cds.go

build-fast: \
	build-api-agent-fast \
	build-client-fast

build-pb: install-protobuf-tools
	@echo "$(ECHO_BEFORE2)Building protobuf$(ECHO_AFTER)"
	mkdir -p internal/api/v1/cdspb
	rm -f internal/api/v1/cdspb/*.pb.go
	protoc --proto_path=. \
		--go_out=internal/api/v1 --go_opt=module=github.com/amadeusitgroup/cds \
		--go-grpc_out=internal/api/v1 --go-grpc_opt=module=github.com/amadeusitgroup/cds \
		$$(find internal/api/v1 -name '*.proto' | LC_ALL=C sort)
test:
	@echo "$(ECHO_BEFORE2)Executing tests$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go test ./... -v

test-fast:
	@echo "$(ECHO_BEFORE2)Executing fast package tests$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go test ./internal/command ./internal/agent ./internal/db ./internal/containerconf ./internal/engine ./internal/bootstrap ./internal/shexec

gen-coverage:
	@echo "$(ECHO_BEFORE2)Executing coverage$(ECHO_AFTER)"
	CDS_CONFIG_PATH=$(CDS_CONFIG_PATH) go test ./... -coverprofile=coverage.out

coverage: gen-coverage
	@echo "$(ECHO_BEFORE2)Generating coverage report$(ECHO_AFTER)"
	go tool cover -html=coverage.out

# Include delivery targets
include makefile.delivery
