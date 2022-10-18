GO ?= go
GOFMT ?= gofmt "-s"
GO_VERSION=$(shell $(GO) version | cut -c 14- | cut -d' ' -f1 | cut -d'.' -f2)
PACKAGES ?= $(shell $(GO) list ./...)
VETPACKAGES ?= $(shell $(GO) list ./...)
GOFILES := $(shell find . -name "*.go" -type f -not -path "./vendor/*")
TESTFOLDER := $(shell $(GO) list ./...)
TESTTAGS ?=
GCFLAGS ?= "-gcflags=all=-l"
GCFLAGS_ESCAPE ?= "-gcflags=-m -l"
ESCAPE_PATH ?= ""

.PHONY: check
check: fmt-check misspell-check golangci cover

.PHONY: test
test:
	echo "mode: count" > coverage.out
	for d in $(TESTFOLDER); do \
		$(GO) test ${GCFLAGS} $(TESTTAGS) -v -covermode=count -coverprofile=profile.out $$d > tmp.out; \
		cat tmp.out; \
		if grep -q "^--- FAIL" tmp.out; then \
			rm tmp.out; \
			exit 1; \
		elif grep -q "build failed" tmp.out; then \
			rm tmp.out; \
			exit 1; \
		elif grep -q "setup failed" tmp.out; then \
			rm tmp.out; \
			exit 1; \
		fi; \
		if [ -f profile.out ]; then \
			cat profile.out | grep -v "mode:" >> coverage.out; \
			rm profile.out; \
		fi; \
	done

.PHONY: cover
cover:
	@$(GO) test ${GCFLAGS} -cover ./...
	@echo "cover done"

.PHONY: fmt
fmt:
	$(GOFMT) -w $(GOFILES)
	@echo "fmt done"

.PHONY: fmt-check
fmt-check:
	@diff=$$($(GOFMT) -s -d $(GOFILES)); \
	if [ -n "$$diff" ]; then \
		echo "Please run 'make fmt' and commit the result:"; \
		echo "$${diff}"; \
		exit 1; \
	fi;
	@echo "fmt-check done"

vet:
	$(GO) vet $(VETPACKAGES)
	@echo "vet done"

.PHONY: lint
lint:
	@hash golint > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) install golang.org/x/lint/golint@latest; \
	fi
	for PKG in $(PACKAGES); do golint -set_exit_status $$PKG || exit 1; done;
	@echo "lint done"

.PHONY: misspell-check
misspell-check:
	@hash misspell > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) install github.com/client9/misspell/cmd/misspell@latest; \
	fi
	@misspell -error $(GOFILES)
	@echo "misspell-check done"

.PHONY: misspell
misspell:
	@hash misspell > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) install github.com/client9/misspell/cmd/misspell@latest; \
	fi
	@misspell -w $(GOFILES)
	@echo "misspell done"

.PHONY: tools
tools:
	@if [ $(GO_VERSION) -gt 15 ]; then \
		$(GO) install golang.org/x/lint/golint@latest; \
		$(GO) install github.com/client9/misspell/cmd/misspell@latest; \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.49.0; \
	elif [ $(GO_VERSION) -lt 16 ]; then \
		$(GO) install golang.org/x/lint/golint; \
		$(GO) install github.com/client9/misspell/cmd/misspell; \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.49.0; \
	fi

.PHONY: golangci-lint
golangci-lint:
	@hash golangci-lint > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.49.0; \
	fi
	@for FILE in $(GOFILES); do golangci-lint run -set_exit_status $$FILE || exit 1; done;
	@echo "golangci-lint done"

.PHONY: golangci
golangci:
	@hash golangci-lint > /dev/null 2>&1; if [ $$? -ne 0 ]; then \
		$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.49.0; \
	fi
	@golangci-lint run ./... || exit 0;
	@echo "golangci done"

.PHONY: mod
mod:
	@go mod tidy

.PHONY: escape
escape:
	@$(GO) test ${GCFLAGS_ESCAPE} -cover ./${ESCAPE_PATH}...
	@echo "escape done"
