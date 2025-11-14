#! /usr/bin/make -f

PROJECT         := go-fdo-server
SOURCE_DIR      := $(CURDIR)/build/package/rpm
SPEC_FILE_NAME  := $(PROJECT).spec
SPEC_FILE       := $(SOURCE_DIR)/$(SPEC_FILE_NAME)
COMMIT          := $(shell git rev-parse HEAD)
COMMIT_SHORT    := $(shell git rev-parse --short HEAD)
VERSION         := $(shell grep 'Version:' $(SPEC_FILE) | awk '{printf "%s", $$2}')
ARCH            := $(shell uname -m)

# Default target
all: build test

# Build the Go project
.PHONY: build
build: tidy fmt vet
	CGO_ENABLED=1 go build

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

#
# Generating sources and vendor tar files
#
SOURCE_TARBALL_FILENAME    := go-fdo-server-$(COMMIT).tar.gz
SOURCE_TARBALL             := $(SOURCE_DIR)/${SOURCE_TARBALL_FILENAME}
$(SOURCE_TARBALL):
	git archive --prefix=go-fdo-server-$(COMMIT)/ --format=tar.gz HEAD > $(SOURCE_TARBALL)

.PHONY: source-tarball
source-tarball: $(SOURCE_TARBALL)

GO_VENDOR_TOOLS_FILE_NAME  := go-vendor-tools.toml
GO_VENDOR_TOOLS_FILE       := $(SOURCE_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
VENDOR_TARBALL_FILENAME    := go-fdo-server-$(COMMIT)-vendor.tar.bz2
VENDOR_TARBALL             := $(SOURCE_DIR)/$(VENDOR_TARBALL_FILENAME)
$(VENDOR_TARBALL):
	rm -rf vendor; \
	command -v go_vendor_archive || sudo dnf install -y go-vendor-tools python3-tomlkit askalono-cli; \
	go_vendor_archive create --config $(GO_VENDOR_TOOLS_FILE) --write-config --output $(VENDOR_TARBALL) .; \
	rm -rf vendor;

.PHONY: vendor-tarball
vendor-tarball: $(VENDOR_TARBALL)

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

RPMBUILD_TOP_DIR                      := $(CURDIR)/rpmbuild
RPMBUILD_BUILD_DIR                    := $(RPMBUILD_TOP_DIR)/build
RPMBUILD_RPMS_DIR                     := $(RPMBUILD_TOP_DIR)/rpms
RPMBUILD_SPECS_DIR                    := $(RPMBUILD_TOP_DIR)/specs
RPMBUILD_SOURCES_DIR                  := $(RPMBUILD_TOP_DIR)/sources
RPMBUILD_SRPMS_DIR                    := $(RPMBUILD_TOP_DIR)/srpms
RPMBUILD_BUILD_DIR                    := $(RPMBUILD_TOP_DIR)/build
RPMBUILD_BUILDROOT_DIR                := $(RPMBUILD_TOP_DIR)/buildroot
RPMBUILD_GOLANG_VENDOR_TOOLS_FILE     := $(RPMBUILD_SOURCES_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
RPMBUILD_SPECFILE                     := $(RPMBUILD_SPECS_DIR)/go-fdo-server-$(COMMIT).spec
RPMBUILD_TARBALL                      := $(RPMBUILD_SOURCES_DIR)/$(SOURCE_TARBALL_FILENAME)
RPMBUILD_VENDOR_TARBALL               := ${RPMBUILD_SOURCES_DIR}/$(VENDOR_TARBALL_FILENAME)
RPMBUILD_GROUP_FILE                   := $(RPMBUILD_SOURCES_DIR)/$(GROUP_FILE_NAME)
RPMBUILD_MANUFACTURER_USER_FILE       := $(RPMBUILD_SOURCES_DIR)/$(MANUFACTURER_USER_FILE_NAME)
RPMBUILD_RENDEZVOUS_USER_FILE         := $(RPMBUILD_SOURCES_DIR)/$(RENDEZVOUS_USER_FILE_NAME)
RPMBUILD_OWNER_USER_FILE              := $(RPMBUILD_SOURCES_DIR)/$(OWNER_USER_FILE_NAME)
RPMBUILD_SRPM_FILE                    := $(RPMBUILD_SRPMS_DIR)/$(PROJECT)-$(VERSION)-git$(COMMIT_SHORT).src.rpm
RPMBUILD_RPM_FILE                     := $(RPMBUILD_RPMS_DIR)/$(ARCH)/$(PROJECT)-$(VERSION)-git$(COMMIT_SHORT).$(ARCH).rpm


$(RPMBUILD_SPECFILE):
	mkdir -p $(RPMBUILD_SPECS_DIR)
	sed -e "s/^%global commit\(\s*\).*/%global commit\1$(COMMIT)/;" \
		  -e "s/^Release:\(\s*\).*/Release:\1git$(COMMIT_SHORT)/;" \
	    $(SPEC_FILE) > $(RPMBUILD_SPECFILE)

$(RPMBUILD_TARBALL): $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	mkdir -p $(RPMBUILD_SOURCES_DIR)
	cp $(SOURCE_TARBALL) $(RPMBUILD_TARBALL)
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
