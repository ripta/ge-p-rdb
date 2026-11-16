SHELL := bash -o pipefail
.DEFAULT_GOAL := help
.DELETE_ON_ERROR:
.SUFFIXES:
MAKEFLAGS += --warn-undefined-variables

# Directory for built binaries.
BUILD_DIR ?= build
# Directory holding real P-files for test-samples. See pkg/pfile/pfile_test.go.
SAMPLES ?=
# Space-separated paths, relative to the repo root, for fmt-select. Empty formats nothing.
FILES ?=

.PHONY: help
help: ## Show this help
	@awk 'BEGIN { FS = ":.*## " } \
		/^##@ / { printf "\n%s\n", substr($$0, 5) } \
		/^[a-zA-Z0-9_.\/-]+:([^=]|$$).*## / { printf "  %-24s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

##@ Build

.PHONY: build
build: ## Build pfinfo into BUILD_DIR
	go build -o $(BUILD_DIR)/pfinfo ./cmd/pfinfo

.PHONY: check
check: ## Compile every package without producing binaries
	go build ./...

##@ Test

.PHONY: test
test: ## Run the unit tests
	go test ./...

# The sample tests read real P-files, which are not in the repo.
.PHONY: test-samples
test-samples: ## Run the tests against real P-files, e.g. make test-samples SAMPLES=~/Downloads
	@test -n "$(SAMPLES)" || { echo "set SAMPLES to a directory of P-files"; exit 1; }
	go test -count=1 ./pkg/pfile -samples $(SAMPLES)

##@ Lint

.PHONY: lint
lint: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format all Go files in place
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if any Go file needs formatting
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "needs gofmt:"; echo "$$out"; exit 1; fi

.PHONY: fmt-select
fmt-select: ## Format only the named files, e.g. make fmt-select FILES='cmd/pfinfo/main.go'
	$(if $(filter %.go,$(FILES)),gofmt -w $(filter %.go,$(FILES)))

.PHONY: ci
ci: fmt-check lint check test ## Run what CI runs

##@ Clean

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BUILD_DIR)
