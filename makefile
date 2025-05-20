# define a variable if it is not already defined
GOLANGCI_LINT_VERSION := latest
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
CDS_CONFIG_PATH=${HOME_DIR}/cdstmp

# check that all required variables are set
# vars := GOLANGCI_LINT_VERSION GOPATH GOPRIVATE GOPROXY CDS_CONFIG_PATH
vars := GOLANGCI_LINT_VERSION GOPATH CDS_CONFIG_PATH
$(foreach var, $(vars), $(if $(value $(var)), $(info $(var)=$(value $(var))), $(error $(var) is not set)))

.PHONY: install \
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
	mkdir -p $(TEST_FOLDER) $(CDS_CONFIG_PATH)/.xcds/certs

gencert: init
	CDS_CONFIG_PATH=${HOME_DIR}/cdstmp
	@echo "$(ECHO_BEFORE)Generating certificates$(ECHO_AFTER)"
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

go-tidy:
	@echo "$(ECHO_BEFORE)Executing go mod tidy$(ECHO_AFTER)"
	go mod tidy

lint: install-golangci-lint
	@echo "$(ECHO_BEFORE)Executing lint$(ECHO_AFTER)"
	golangci-lint run ./...

lint-weak: install-golangci-lint
	@echo "$(ECHO_BEFORE)Executing weak lint$(ECHO_AFTER)"
	golangci-lint run ./... --exclude 'is unused'

install-golangci-lint:
	@echo "$(ECHO_BEFORE)Executing install-golangci-lint$(ECHO_AFTER)"
	which golangci-lint || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)

run-api-agent: go-tidy
	@echo "$(ECHO_BEFORE2)Running cds api server$(ECHO_AFTER)"
	go run ./cmd/api-agent/cds-api-server.go start

run-client: go-tidy
	@echo "$(ECHO_BEFORE2)Running cds CLI$(ECHO_AFTER)"
	go run ./cmd/client/cds.go

run-metrics-analyzer: go-tidy
	@echo "$(ECHO_BEFORE2)Running metrics analyzer$(ECHO_AFTER)"
	go run ./cmd/metrics-analyzer/analyzer.go

build-api-agent: go-tidy build-pb
	@echo "$(ECHO_BEFORE2)Building cds api server$(ECHO_AFTER)"
	go build -o cds-api-agent ./cmd/api-agent/cds-api-agent.go

build-client: go-tidy build-pb
	@echo "$(ECHO_BEFORE2)Building cds CLI$(ECHO_AFTER)"
	go build -o cds ./cmd/client/cds.go

build-pb: go-tidy
	@echo "$(ECHO_BEFORE2)Building protobuf$(ECHO_AFTER)"
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/api/v1/*.proto
test:
	@echo "$(ECHO_BEFORE2)Executing tests$(ECHO_AFTER)"
	go test ./... -v

gen-coverage:
	@echo "$(ECHO_BEFORE2)Executing coverage$(ECHO_AFTER)"
	CDS_CONFIG_PATH=${HOME_DIR}/cdstmp go test ./... -coverprofile=coverage.out

coverage: gen-coverage
	@echo "$(ECHO_BEFORE2)Generating coverage report$(ECHO_AFTER)"
	go tool cover -html=coverage.out
