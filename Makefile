#! /usr/bin/make -f

PROJECT         := go-fdo-server
ARCH            := $(shell uname -m)
COMMIT_SHORT    := $(shell git rev-parse --short HEAD)
SOURCE_DIR      := $(CURDIR)/build/package/rpm
SPEC_FILE_NAME  := $(PROJECT).spec
SPEC_FILE       := $(SOURCE_DIR)/$(SPEC_FILE_NAME)
VERSION         := $(shell grep 'Version:' $(SPEC_FILE) | awk '{printf "%s", $$2}')
GOFLAGS         ?=
COVERDIR        ?= $(CURDIR)/test/coverage
GOCOVERDIR      ?= $(COVERDIR)/integration


# Default target
all: build test

# Build the Go project
.PHONY: build
build: generate tidy fmt vet
	go build $(GOFLAGS) -ldflags="-X github.com/fido-device-onboard/go-fdo-server/internal/version.VERSION=${VERSION}"

.PHONY: oapi-codegen
oapi-codegen:
	@echo "Installing oapi-codegen..."
	go get -tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest

.PHONY: generate
generate: oapi-codegen
	go generate ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: vendor
vendor:
	go mod vendor

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test -v ./...

.PHONY: man
man:
	go run ./internal/tools/docgen -format man

.PHONY: shfmt
shfmt:
	shfmt -i 2 -ci -w .

.PHONY: test-coverage
test-coverage: SHELL := /usr/bin/env bash
test-coverage:
	rm -rf "$(COVERDIR)"
	mkdir -p "$(GOCOVERDIR)"
	go test -coverpkg=./... -coverprofile="$(COVERDIR)/unit.out" -covermode=atomic ./...
	export GOCOVERDIR="$(GOCOVERDIR)"; \
	export GOFLAGS="-cover -covermode=atomic"; \
	set -e; \
	for t in test/ci/test-*.sh; do \
		echo "=== RUNNING: $$t ==="; \
		($$t); \
	done; \
	go tool covdata textfmt -i="$(GOCOVERDIR)" -o="$(COVERDIR)/integration.out"
	go install github.com/wadey/gocovmerge@latest
	gocovmerge "$(COVERDIR)/unit.out" "$(COVERDIR)/integration.out" > "$(COVERDIR)/coverage.out"
	go tool cover -html="$(COVERDIR)/coverage.out" -o "$(COVERDIR)/coverage.html"

#
# Generating sources and vendor tar files
#
SOURCE_TARBALL_FILENAME    := go-fdo-server-$(VERSION).tar.gz
SOURCE_TARBALL             := $(SOURCE_DIR)/${SOURCE_TARBALL_FILENAME}
$(SOURCE_TARBALL):
	git archive --prefix=go-fdo-server-$(VERSION)/ --format=tar.gz HEAD > $(SOURCE_TARBALL)

.PHONY: source-tarball
source-tarball: $(SOURCE_TARBALL)

GO_VENDOR_TOOLS_FILE_NAME  := go-vendor-tools.toml
GO_VENDOR_TOOLS_FILE       := $(SOURCE_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
VENDOR_TARBALL_FILENAME    := go-fdo-server-$(VERSION)-vendor.tar.bz2
VENDOR_TARBALL             := $(SOURCE_DIR)/$(VENDOR_TARBALL_FILENAME)

.PHONY: install-go-vendor-tools
install-go-vendor-tools:
	command -v go_vendor_archive || sudo dnf install -y go-vendor-tools python3-tomlkit askalono-cli go-rpm-macros

$(VENDOR_TARBALL): install-go-vendor-tools
	rm -rf vendor; \
	go_vendor_archive create --config $(GO_VENDOR_TOOLS_FILE) --write-config --output $(VENDOR_TARBALL) .; \
	rm -rf vendor;

.PHONY: vendor-tarball
vendor-tarball: $(VENDOR_TARBALL)

.PHONY: update-rpm-licensing
update-rpm-licensing: install-go-vendor-tools $(SPEC_FILE) $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	go_vendor_license --config $(GO_VENDOR_TOOLS_FILE) --path $(SPEC_FILE) report --update-spec --autofill=auto

#
# Building packages
#
# The following rules build FDO packages from the current HEAD commit,
# based on the spec file in build/package/rpm directory. The resulting packages
# have the commit hash in their version, so that they don't get overwritten when calling
# `make rpm` again after switching to another branch or adding new commits.
#
# All resulting files (spec files, source rpms, rpms) are written into
# ./rpmbuild, using rpmbuild's usual directory structure (in lowercase).
#

GROUP_FILE_NAME                       := go-fdo-server-group.conf
GROUP_FILE                            := $(SOURCE_DIR)/$(GROUP_FILE_NAME)

MANUFACTURER_USER_FILE_NAME           := go-fdo-server-manufacturer-user.conf
MANUFACTURER_USER_FILE                := $(SOURCE_DIR)/$(MANUFACTURER_USER_FILE_NAME)

RENDEZVOUS_USER_FILE_NAME             := go-fdo-server-rendezvous-user.conf
RENDEZVOUS_USER_FILE                  := $(SOURCE_DIR)/$(RENDEZVOUS_USER_FILE_NAME)

OWNER_USER_FILE_NAME                  := go-fdo-server-owner-user.conf
OWNER_USER_FILE                       := $(SOURCE_DIR)/$(OWNER_USER_FILE_NAME)

RPMBUILD_VERSION                      := $(VERSION).git$(COMMIT_SHORT)
RPMBUILD_TOP_DIR                      := $(CURDIR)/rpmbuild
RPMBUILD_BUILD_DIR                    := $(RPMBUILD_TOP_DIR)/build
RPMBUILD_RPMS_DIR                     := $(RPMBUILD_TOP_DIR)/rpms
RPMBUILD_SPECS_DIR                    := $(RPMBUILD_TOP_DIR)/specs
RPMBUILD_SOURCES_DIR                  := $(RPMBUILD_TOP_DIR)/sources
RPMBUILD_SRPMS_DIR                    := $(RPMBUILD_TOP_DIR)/srpms
RPMBUILD_BUILD_DIR                    := $(RPMBUILD_TOP_DIR)/build
RPMBUILD_BUILDROOT_DIR                := $(RPMBUILD_TOP_DIR)/buildroot
RPMBUILD_GOLANG_VENDOR_TOOLS_FILE     := $(RPMBUILD_SOURCES_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
RPMBUILD_SPECFILE                     := $(RPMBUILD_SPECS_DIR)/go-fdo-server-$(RPMBUILD_VERSION).spec
RPMBUILD_TARBALL                      := $(RPMBUILD_SOURCES_DIR)/go-fdo-server-$(RPMBUILD_VERSION).tar.gz
RPMBUILD_VENDOR_TARBALL               := ${RPMBUILD_SOURCES_DIR}/go-fdo-server-$(RPMBUILD_VERSION)-vendor.tar.bz2
RPMBUILD_GROUP_FILE                   := $(RPMBUILD_SOURCES_DIR)/$(GROUP_FILE_NAME)
RPMBUILD_MANUFACTURER_USER_FILE       := $(RPMBUILD_SOURCES_DIR)/$(MANUFACTURER_USER_FILE_NAME)
RPMBUILD_RENDEZVOUS_USER_FILE         := $(RPMBUILD_SOURCES_DIR)/$(RENDEZVOUS_USER_FILE_NAME)
RPMBUILD_OWNER_USER_FILE              := $(RPMBUILD_SOURCES_DIR)/$(OWNER_USER_FILE_NAME)
RPMBUILD_SRPM_FILE                    := $(RPMBUILD_SRPMS_DIR)/$(PROJECT)-$(VERSION)-git$(COMMIT_SHORT).src.rpm
RPMBUILD_RPM_FILE                     := $(RPMBUILD_RPMS_DIR)/$(ARCH)/$(PROJECT)-$(VERSION)-git$(COMMIT_SHORT).$(ARCH).rpm


$(RPMBUILD_SPECFILE):
	mkdir -p $(RPMBUILD_SPECS_DIR)
	sed -e "s/^Version:\(\s*\).*/Version:\1$(RPMBUILD_VERSION)/;" \
	    $(SPEC_FILE) > $(RPMBUILD_SPECFILE)

$(RPMBUILD_TARBALL): $(VENDOR_TARBALL)
	mkdir -p $(RPMBUILD_SOURCES_DIR)
	git archive --prefix=go-fdo-server-$(RPMBUILD_VERSION)/ --format=tar.gz HEAD > $(RPMBUILD_TARBALL)
	cp $(VENDOR_TARBALL) $(RPMBUILD_VENDOR_TARBALL);

$(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE):
	cp $(GO_VENDOR_TOOLS_FILE) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)

$(RPMBUILD_GROUP_FILE):
	cp $(GROUP_FILE) $(RPMBUILD_GROUP_FILE)

$(RPMBUILD_MANUFACTURER_USER_FILE):
	cp $(MANUFACTURER_USER_FILE) $(RPMBUILD_MANUFACTURER_USER_FILE)

$(RPMBUILD_RENDEZVOUS_USER_FILE):
	cp $(RENDEZVOUS_USER_FILE) $(RPMBUILD_RENDEZVOUS_USER_FILE)

$(RPMBUILD_OWNER_USER_FILE):
	cp $(OWNER_USER_FILE) $(RPMBUILD_OWNER_USER_FILE)

$(RPMBUILD_SRPM_FILE): $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE) $(RPMBUILD_GROUP_FILE) $(RPMBUILD_MANUFACTURER_USER_FILE) $(RPMBUILD_RENDEZVOUS_USER_FILE) $(RPMBUILD_OWNER_USER_FILE)
	command -v rpmbuild || sudo dnf install -y rpm-build ; \
	rpmbuild -bs \
		--define "_topdir $(RPMBUILD_TOP_DIR)" \
		--define "_rpmdir $(RPMBUILD_RPMS_DIR)" \
		--define "_sourcedir $(RPMBUILD_SOURCES_DIR)" \
		--define "_specdir $(RPMBUILD_SPECS_DIR)" \
		--define "_srcrpmdir $(RPMBUILD_SRPMS_DIR)" \
		--define "_builddir $(RPMBUILD_BUILD_DIR)" \
		--define "_buildrootdir $(RPMBUILD_BUILDROOT_DIR)" \
		$(RPMBUILD_SPECFILE)

.PHONY: srpm
srpm: $(RPMBUILD_SRPM_FILE)

$(RPMBUILD_RPM_FILE): $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE) $(RPMBUILD_GROUP_FILE) $(RPMBUILD_MANUFACTURER_USER_FILE) $(RPMBUILD_RENDEZVOUS_USER_FILE) $(RPMBUILD_OWNER_USER_FILE)
	command -v rpmbuild || sudo dnf install -y rpm-build ; \
	sudo dnf builddep -y $(RPMBUILD_SPECFILE)
	rpmbuild -bb \
		--define "_topdir $(RPMBUILD_TOP_DIR)" \
		--define "_rpmdir $(RPMBUILD_RPMS_DIR)" \
		--define "_sourcedir $(RPMBUILD_SOURCES_DIR)" \
		--define "_specdir $(RPMBUILD_SPECS_DIR)" \
		--define "_srcrpmdir $(RPMBUILD_SRPMS_DIR)" \
		--define "_builddir $(RPMBUILD_BUILD_DIR)" \
		--define "_buildrootdir $(RPMBUILD_BUILDROOT_DIR)" \
		$(RPMBUILD_SPECFILE)

.PHONY: rpm
rpm: $(RPMBUILD_RPM_FILE)

.PHONY: clean
clean:
	rm -rf $(RPMBUILD_TOP_DIR)
	rm -rf $(SOURCE_DIR)/go-fdo-server-*.tar.{gz,bz2}

.PHONY: fdo-openapi-ui
fdo-openapi-ui:
	container_cmd=`command -v podman`; \
	[ -n "$${container_cmd}" ] || container_cmd=`command -v docker`; \
	[ -n "$${container_cmd}" ] || { echo "No container runtime found" ; exit 1; }; \
	$${container_cmd} rm --force fdo-openapi-ui; \
	$${container_cmd} run --rm --name fdo-openapi-ui -d -p 9080:8080 -v ./api:/usr/share/nginx/html/api:z -e URLS='[{"url": "/api/manufacturer/openapi.yaml", "name": "Manufacturer API"}, {"url": "/api/rendezvous/openapi.yaml", "name": "Rendezvous API"}, {"url": "/api/owner/openapi.yaml", "name": "Owner API"} ]' docker.swagger.io/swaggerapi/swagger-ui; \
	until curl -s -o /dev/null http://127.0.0.1:9080; do \
	  echo "Waiting for swagger-ui to be ready..."; \
		sleep 1; \
	done; \
	open_url_cmd=`command -v xdg-open`; [ -n "$${open_url_cmd}" ] || open_url_cmd=`command -v open`; $${open_url_cmd} http://127.0.0.1:9080
