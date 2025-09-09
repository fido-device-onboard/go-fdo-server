#! /usr/bin/make -f
COMMIT = $(shell git rev-parse --short HEAD)
DATE = $(shell date "+%Y%m%d" )
VERSION = git$(DATE).$(COMMIT)

# Build the Go project
.PHONY: build
build: tidy fmt vet
	CGO_ENABLED=0 go build

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
SOURCE_DIR=$(CURDIR)/build/package/rpm

SOURCE_TARBALL_FILENAME=go-fdo-server-$(VERSION).tar.gz
SOURCE_TARBALL=$(SOURCE_DIR)/${SOURCE_TARBALL_FILENAME}
$(SOURCE_TARBALL):
	git archive --prefix=go-fdo-server-$(VERSION)/ --format=tar.gz HEAD > $(SOURCE_TARBALL)

.PHONY: source-tarball
source-tarball: $(SOURCE_TARBALL)

GO_VENDOR_TOOLS_FILE_NAME=go-vendor-tools.toml
GO_VENDOR_TOOLS_FILE=$(SOURCE_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
VENDOR_TARBALL_FILENAME=go-fdo-server-$(VERSION)-vendor.tar.gz
VENDOR_TARBALL=$(SOURCE_DIR)/$(VENDOR_TARBALL_FILENAME)

$(VENDOR_TARBALL):
	rm -rf vendor; \
	command -v go_vendor_archive || sudo dnf install -y go-vendor-tools python3-tomlkit; \
	go_vendor_archive create --compression gz --config $(GO_VENDOR_TOOLS_FILE) --write-config --output $(VENDOR_TARBALL) .; \
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

SPEC_FILE_NAME=go-fdo-server.spec
SPEC_FILE=$(SOURCE_DIR)/$(SPEC_FILE_NAME)
RPMBUILD_TOP_DIR=$(CURDIR)/rpmbuild
RPMBUILD_BUILD_DIR=$(RPMBUILD_TOP_DIR)/build
RPMBUILD_RPMS_DIR=$(RPMBUILD_TOP_DIR)/rpms
RPMBUILD_SPECS_DIR=$(RPMBUILD_TOP_DIR)/specs
RPMBUILD_SOURCES_DIR=$(RPMBUILD_TOP_DIR)/sources
RPMBUILD_SRPMS_DIR=$(RPMBUILD_TOP_DIR)/srpms
RPMBUILD_BUILD_DIR=$(RPMBUILD_TOP_DIR)/build
RPMBUILD_BUILDROOT_DIR=$(RPMBUILD_TOP_DIR)/buildroot
RPMBUILD_GOLANG_VENDOR_TOOLS_FILE=$(RPMBUILD_SOURCES_DIR)/$(GO_VENDOR_TOOLS_FILE_NAME)
RPMBUILD_SPECFILE=$(RPMBUILD_SPECS_DIR)/$(SPEC_FILE_NAME)
RPMBUILD_TARBALL=$(RPMBUILD_SOURCES_DIR)/$(SOURCE_TARBALL_FILENAME)
RPMBUILD_VENDOR_TARBALL=${RPMBUILD_SOURCES_DIR}/$(VENDOR_TARBALL_FILENAME)

$(RPMBUILD_SPECFILE):
	mkdir -p $(RPMBUILD_SPECS_DIR)
	sed -e "s/^Version:.*/Version:        $(VERSION)/;" \
		  -e "s/^Source0:.*/Source0:        go-fdo-server-$(VERSION).tar.gz/;" \
	    $(SPEC_FILE) > $(RPMBUILD_SPECFILE)

$(RPMBUILD_TARBALL): $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	mkdir -p $(RPMBUILD_SOURCES_DIR)
	mv $(SOURCE_TARBALL) $(RPMBUILD_TARBALL)
	mv $(VENDOR_TARBALL) $(RPMBUILD_VENDOR_TARBALL);

$(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE):
	cp $(GO_VENDOR_TOOLS_FILE) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)

.PHONY: srpm
srpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)
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

.PHONY: rpm
rpm: $(RPMBUILD_SPECFILE) $(RPMBUILD_TARBALL) $(RPMBUILD_GOLANG_VENDOR_TOOLS_FILE)
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

#
# Packit target
#

.PHONY: packit-create-archive
packit-create-archive: $(SOURCE_TARBALL) $(VENDOR_TARBALL)
	ls -1 $(SOURCE_TARBALL)

.PHONY: clean
clean:
	rm -rf $(RPMBUILD_TOP_DIR)
	rm -rf $(SOURCE_DIR)/go-fdo-server-*.tar.gz

# Default target
all: build test
