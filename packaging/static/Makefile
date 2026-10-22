# Copyright (c) 2026 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

NULL :=
MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
REQUIRED_VERSION := 4.4

ifneq ($(REQUIRED_VERSION),$(firstword $(sort $(REQUIRED_VERSION) $(MAKE_VERSION))))
    $(error Unsupported Make version: $(MAKE_VERSION). Please use GNU Make $(REQUIRED_VERSION) or higher)
endif

BUILDARCH ?= $(shell uname -m)

# Paths
SRCDIR := $(abspath $(MAKEFILE_DIR)../..)
STAGING ?= $(MAKEFILE_DIR)/staging
CACHE ?= $(MAKEFILE_DIR)/cache
WORKDIR ?= $(MAKEFILE_DIR)/work

VERSION ?= $(shell cat $(SRCDIR)/packaging/version)

NETDATA_SOURCE_DIR := /netdata
NETDATA_PREFIX := opt/netdata

TARGET_STAGING := $(STAGING)/$(BUILDARCH)
TARGET_WORKDIR := $(WORKDIR)/$(BUILDARCH)
TARGET_STAMPS := $(WORKDIR)/$(BUILDARCH)/stamps
TARGET_INSTALL_BASE := $(TARGET_STAGING)/install
TARGET_INSTALL := $(TARGET_INSTALL_BASE)/$(NETDATA_PREFIX)
TARGET_CACHE_ROOT := $(CACHE)/$(BUILDARCH)

ifdef DEVTEST
    TARGET_CACHE := $(TARGET_CACHE_ROOT)/DEVTEST
else
    TARGET_CACHE := $(TARGET_CACHE_ROOT)/$(shell $(MAKEFILE_DIR)/scripts/cache-key.sh $(MAKEFILE_DIR))
endif

DOCKER_STAGING := /staging
DOCKER_WORKDIR := /work
DOCKER_INSTALL_BASE := /staging/install
DOCKER_INSTALL := $(DOCKER_INSTALL_BASE)/$(NETDATA_PREFIX)

# Docker configuration

DOCKER := $(or $(shell command -v docker 2>/dev/null),$(shell command -v podman 2>/dev/null))

ifeq ($(strip $(DOCKER)),)
    $(error Either Docker or Podman are required for static builds.)
endif

BUILDER := netdata/static-builder:v1
QEMU_SETUP := tonistiigi/binfmt:master
DOCKER_ARGS := $(DOCKER_ARGS) --rm -i \
               -a stdin -a stdout -a stderr \
               -v "$(SRCDIR):$(NETDATA_SOURCE_DIR):rw" \
               -v "$(abspath $(TARGET_STAGING)):$(DOCKER_STAGING):rw" \
               -v "$(abspath $(TARGET_WORKDIR)):$(DOCKER_WORKDIR):rw" \
               $(if $(GITHUB_ACTIONS),-e GITHUB_ACTIONS=$(GITHUB_ACTIONS)) \
               $(if $(EXTRA_INSTALL_FLAGS),-e EXTRA_INSTALL_FLAGS="$(EXTRA_INSTALL_FLAGS)") \
               $(if $(DEBUG_BUILD_INFRA),-e DEBUG_BUILD_INFRA=1) \
               $(NULL)

# component configuration

define set_paths
	$(1)_CACHE_PATH := $$(TARGET_CACHE)/$(2)/$$($(1)_VERSION)
	$(1)_CACHE_HIT := $$(TARGET_STAMPS)/$(2).cache.hit
	$(1)_DIR := $$(TARGET_STAMPS)/$(2).dir.stamp
	$(1)_CONFIG := $$(TARGET_STAMPS)/$(2).config.stamp
	$(1)_BUILD := $$(TARGET_STAMPS)/$(2).build.stamp
	$(1)_STAGING := $$(TARGET_STAMPS)/$(2).staging.stamp
	$(1)_DOCKER_STAGING := $$(DOCKER_STAGING)/$(2)
	$(1)_LIBS := $$($(1)_DOCKER_STAGING)/lib
	$(1)_INCLUDE := $$($(1)_DOCKER_STAGING)/include
	$(1)_PKGCONFIG_PATH = $$($(1)_LIBS)/pkgconfig
endef

OPENSSL_VERSION := openssl-3.6.0
OPENSSL_SOURCE := https://github.com/openssl/openssl
$(eval $(call set_paths,OPENSSL,openssl))
OPENSSL_CFLAGS = $(TUNING_FLAGS) -fno-lto -pipe
OPENSSL_LDFLAGS := -static

LIBNFACCT_VERSION := 1.0.3
LIBNFACCT_SOURCE := https://www.netfilter.org/projects/libnetfilter_acct/files/libnetfilter_acct-$(LIBNFACCT_VERSION).tar.bz2
LIBNFACCT_ARCHIVE := $(TARGET_WORKDIR)/libnetfilter_acct-$(LIBNFACCT_VERSION).tar.bz2
LIBNFACCT_HASH := 4250ceef3efe2034f4ac05906c3ee427db31b9b0a2df41b2744f4bf79a959a1a
$(eval $(call set_paths,LIBNFACCT,libnetfilter_acct))
LIBNFACCT_CFLAGS = $(TUNING_FLAGS) -static -I/usr/lib/libmnl -pipe
LIBNFACCT_LDFLAGS := -static -L/usr/lib -lmnl

CURL_VERSION := curl-8_20_0
CURL_SOURCE := https://github.com/curl/curl
$(eval $(call set_paths,CURL,curl))
CURL_CFLAGS = $(TUNING_FLAGS) -I$(OPENSSL_INCLUDE) -pipe
CURL_LDFLAGS := -static -L$(OPENSSL_LIBS)

BASH_VERSION := 5.3
BASH_SOURCE := http://ftp.gnu.org/gnu/bash/bash-$(BASH_VERSION).tar.gz
BASH_ARCHIVE := $(TARGET_WORKDIR)/bash-$(BASH_VERSION).tar.gz
BASH_HASH := 0d5cd86965f869a26cf64f4b71be7b96f90a3ba8b3d74e27e8e9d9d5550f31ba
$(eval $(call set_paths,BASH,bash))
BASH_CFLAGS = $(TUNING_FLAGS) -pipe
BASH_LDFLAGS :=

IOPING_VERSION := 1.3
IOPING_SOURCE := https://github.com/koct9i/ioping/archive/refs/tags/v$(IOPING_VERSION).tar.gz
IOPING_ARCHIVE := $(TARGET_WORKDIR)/ioping-$(IOPING_VERSION).tar.gz
IOPING_HASH := 7aa48e70aaa766bc112dea57ebbe56700626871052380709df3a26f46766e8c8
$(eval $(call set_paths,IOPING,ioping))
IOPING_CFLAGS = $(TUNING_FLAGS) -static -pipe
IOPING_LDFLAGS :=

# Netdata build config
NETDATA_BUILD_DEPS := $(OPENSSL_STAGING) \
                      $(CURL_STAGING) \
                      $(LIBNFACCT_STAGING) \
                      $(NULL)
$(eval $(call set_paths,NETDATA,netdata))
NETDATA_BUILD_DIR := $(DOCKER_WORKDIR)/netdata/build
NETDATA_INSTALL := $(TARGET_STAMPS)/netdata.install.stamp
NETDATA_CFLAGS = $(TUNING_FLAGS) -pipe -O2 -funroll-loops -I$(OPENSSL_INCLUDE) -I$(LIBNFACCT_INCLUDE) -I$(CURL_INCLUDE)
NETDATA_LDFLAGS := -Wl,--gc-sections -L$(OPENSSL_LIBS) -L$(LIBNFACCT_LIBS) -L$(CURL_LIBS) -L/usr/lib -lmnl -lzstd
NETDATA_RUSTFLAGS := -C target-feature=+crt-static
NETDATA_PKGCONFIG_PATH := $(OPENSSL_PKGCONFIG_PATH):$(LIBNFACCT_PKGCONFIG_PATH):$(CURL_PKGCONFIG_PATH)
NETDATA_CMAKE_OPTS := -DSTATIC_BUILD=On \
                      -DCMAKE_INSTALL_PREFIX=/$(NETDATA_PREFIX) \
                      -DCMAKE_INTERPROCEDURAL_OPTIMIZATION=$(if $(DEVTEST),Off,On) \
                      -DUSE_LTO=$(if $(DEVTEST),Off,On) \
                      -DENABLE_LIBBACKTRACE=$(if $(DEVTEST),Off,On) \
                      -DENABLE_BUNDLED_PROTOBUF=On \
                      -DENABLE_EXPORTER_MONGODB=Off \
                      -DENABLE_PLUGIN_CUPS=Off \
                      -DENABLE_PLUGIN_FREEIPMI=Off \
                      -DENABLE_PLUGIN_SYSTEMD_UNITS=Off \
                      -DENABLE_PLUGIN_XENSTAT=Off \
                      $(NULL)

# archive build options
ARCHIVE_DEPS := $(NETDATA_INSTALL) \
                curl \
                bash \
                ioping \
                tests \
                makeself.lsm \
                makeself-license.txt \
                makeself-help-header.txt \
                makeself/makeself.sh \
                $(TARGET_STAMPS)/ca-certs.stamp \
                $(TARGET_INSTALL)/etc/.install-type \
                $(TARGET_INSTALL)/var/cache/netdata/.keep \
                $(TARGET_INSTALL)/var/lib/netdata/.keep \
                $(TARGET_INSTALL)/var/log/netdata/.keep \
                $(TARGET_INSTALL)/system/functions.sh \
                $(TARGET_INSTALL)/system/post-installer.sh \
                $(TARGET_INSTALL)/system/install-or-update.sh \
                $(NULL)
MAKESELF_OPTS = --notemp \
                --needroot \
                --sha256 \
                --lsm $(MAKEFILE_DIR)/makeself.lsm \
                --license $(MAKEFILE_DIR)/makeself-license.txt \
                --help-header $(MAKEFILE_DIR)/makeself-help-header.txt \
                --target /$(NETDATA_PREFIX) \
                $(TARGET_INSTALL) \
                $@ \
                "Netdata, X-Ray Vision for your infrastructure" \
                ./system/post-installer.sh \
                $(NULL)

ifneq ($(strip $(TOPOLOGY_STOCK_DIR)),)
    DOCKER_ARGS := $(DOCKER_ARGS) \
                   -v "$(abspath $(TOPOLOGY_STOCK_DIR)):/topology-stock:ro" \
                   -e "NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR=/topology-stock" \
                   $(NULL)
    NETDATA_CMAKE_OPTS := $(NETDATA_CMAKE_OPTS) \
                          -DNETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR=/topology-stock \
                          $(NULL)
endif

# Target architecture setup

ifeq ($(BUILDARCH),x86_64)
    PLATFORM := linux/amd64
    QEMU_ARCH := x86_64
    ELF_MACHINE := X86-64
    ELF_CLASS := ELF64
    TUNING_FLAGS := -march=x86-64
    DOCKER_ARGS := $(DOCKER_ARGS) \
                   --platform=$(PLATFORM) \
                   -e QEMU_CPU=Nehalem-v2 \
                   -e GOAMD64=v1 -e GOARCH=amd64 \
                   $(NULL)
    NETDATA_CMAKE_OPTS := $(NETDATA_CMAKE_OPTS) \
                          -DENABLE_NETDATA_JOURNAL_FILE_READER=On \
                          -DENABLE_PLUGIN_NETFLOW=On \
                          $(NULL)
else ifeq ($(BUILDARCH),armv6l)
    PLATFORM := linux/arm/v6
    QEMU_ARCH := arm
    ELF_MACHINE := ARM
    ELF_CLASS := ELF32
    TUNING_FLAGS := -march=armv6zk -mtune=arm1176jzf-s
    DOCKER_ARGS := $(DOCKER_ARGS) \
                   --platform=$(PLATFORM) \
                   -e QEMU_CPU=arm1176 \
                   -e GOARM=6 -e GOARCH=arm \
                   $(NULL)
    OPENSSL_CONFIG_TARGET := linux-armv4
    LIBUCONTEXT_ARCH := arm
    NETDATA_CMAKE_OPTS := $(NETDATA_CMAKE_OPTS) \
                          -DENABLE_PLUGIN_EBPF=Off \
                          -DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off \
                          -DENABLE_PLUGIN_OTEL=Off \
                          -DENABLE_PLUGIN_NETFLOW=Off \
                          $(NULL)
else ifeq ($(BUILDARCH),armv7l)
    PLATFORM := linux/arm/v7
    QEMU_ARCH := arm
    ELF_MACHINE := ARM
    ELF_CLASS := ELF32
    TUNING_FLAGS := -march=armv7-a
    DOCKER_ARGS := $(DOCKER_ARGS) \
                   --platform=$(PLATFORM) \
                   -e QEMU_CPU=cortex-a7 \
                   -e GOARM=7 -e GOARCH=arm \
                   $(NULL)
    OPENSSL_CONFIG_TARGET := linux-armv4
    LIBUCONTEXT_ARCH := arm
    NETDATA_CMAKE_OPTS := $(NETDATA_CMAKE_OPTS) \
                          -DENABLE_PLUGIN_EBPF=Off \
                          -DENABLE_NETDATA_JOURNAL_FILE_READER=On \
                          -DENABLE_PLUGIN_NETFLOW=On \
                          $(NULL)
else ifeq ($(BUILDARCH),aarch64)
    PLATFORM := linux/arm64/v8
    QEMU_ARCH := aarch64
    ELF_MACHINE := AArch64
    ELF_CLASS := ELF64
    TUNING_FLAGS := -march=armv8-a
    DOCKER_ARGS := $(DOCKER_ARGS) \
                   --platform=$(PLATFORM) \
                   -e QEMU_CPU=cortex-a53 \
                   -e GOARM64=v8.0 -e GOARCH=arm64 \
                   $(NULL)
    NETDATA_CMAKE_OPTS := $(NETDATA_CMAKE_OPTS) \
                          -DENABLE_NETDATA_JOURNAL_FILE_READER=On \
                          -DENABLE_PLUGIN_NETFLOW=On \
                          $(NULL)
    NETDATA_LDFLAGS := $(NETDATA_LDFLAGS) \
                       -Wl,--fix-cortex-a53-843419 \
                       $(NULL)
else
    $(error Unknown build architecture $(BUILDARCH))
endif

ifneq ($(BUILDARCH),$(shell uname -m))
    QEMU_DEP := /proc/sys/fs/binfmt_misc/qemu-$(QEMU_ARCH)
else
    QEMU_DEP :=
endif

BUILDER_STATUS := $(shell $(MAKEFILE_DIR)/scripts/docker-image-status.sh $(DOCKER) $(BUILDER) $(PLATFORM))

# general rules

.PHONY: clean clean-workdir clean-staging clean-all \
        clear-cache clear-full-cache \
        docker-image docker-shell docker-platform

all: archive-gz archive-zstd

clean: clean-workdir clean-staging
	rm -f makeself.lsm

clean-workdir:
	rm -rf $(TARGET_WORKDIR)

clean-staging:
	rm -rf $(TARGET_STAGING)
	rm -rf $(TARGET_STAMPS)

clean-install:
	rm -rf $(TARGET_INSTALL)
	rm -rf $(TARGET_STAMPS)
	rm -f $(NETDATA_INSTALL)

clear-cache:
	rm -rf $(TARGET_CACHE_ROOT)

clear-full-cache:
	rm -rf $(CACHE)

clean-all: clean-workdir clean-staging clear-cache

$(CACHE) $(CACHE)/CACHEDIR.TAG:
	mkdir -p $(CACHE)
	touch $(CACHE)/CACHEDIR.TAG

$(TARGET_CACHE): | $(CACHE)
	mkdir -p $@

$(TARGET_WORKDIR):
	mkdir -p $@

$(TARGET_STAMPS): | $(TARGET_WORKDIR)
	mkdir -p $@

$(TARGET_STAGING):
	mkdir -p $@

$(TARGET_INSTALL): | $(TARGET_STAGING)
	mkdir -p $@

dirs: $(TARGET_CACHE) $(TARGET_WORKDIR) $(TARGET_STAMPS) $(TARGET_STAGING) $(TARGET_INSTALL)

/proc/sys/fs/binfmt_misc/qemu-$(QEMU_ARCH): /proc/sys/fs/binfmt_misc
	$(DOCKER) run --rm --privileged $(QEMU_SETUP) --install "$(QEMU_ARCH)" ; \

docker-image:
	[ "$(BUILDER_STATUS)" != "MISMATCH" ] || "$(DOCKER)" image rm "$(BUILDER)"
	[ "$(BUILDER_STATUS)" != "OK" ] || $(DOCKER) pull --platform "$(PLATFORM)" "$(BUILDER)"

docker-shell: $(QEMU_DEP) $(DOCKER_IMAGE_DEP)
	+$(DOCKER) run $(DOCKER_ARGS) $(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) -it $(BUILDER) /bin/bash

docker-platform:
	@echo "$(PLATFORM)"

prepare: | $(QEMU_DEP) docker-image dirs

# OpenSSL rules

$(OPENSSL_DIR): prepare
	rm -rf $(TARGET_WORKDIR)/openssl
	if [ -d $(OPENSSL_CACHE_PATH)/openssl ]; then \
		cp -a --reflink=auto $(OPENSSL_CACHE_PATH)/openssl $(TARGET_WORKDIR) && \
		touch $(OPENSSL_CACHE_HIT) ; \
	else \
		rm -f $(OPENSSL_CACHE_HIT) && \
		git clone --quiet --branch "$(OPENSSL_VERSION)" --single-branch --depth 1 "$(OPENSSL_SOURCE)" "$(TARGET_WORKDIR)/openssl" ; \
	fi
	touch $@

$(OPENSSL_CONFIG): prepare $(OPENSSL_DIR)
	+if [ ! -f $(OPENSSL_CACHE_HIT) ]; then \
		sed -i "s/disable('static', 'pic', 'threads');/disable('static', 'pic');/" $(TARGET_WORKDIR)/openssl/Configure && \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(OPENSSL_CFLAGS)" \
			-e CXXFLAGS="$(OPENSSL_CFLAGS)" \
			-e LDFLAGS="-static" \
			-e PKG_CONFIG="pkg-config --static" \
			-w $(DOCKER_WORKDIR)/openssl \
			$(BUILDER) \
			/bin/bash ./config -static threads no-tests enable-quic \
				--prefix=$(OPENSSL_DOCKER_STAGING) \
				--openssldir=/opt/netdata/etc/ssl \
				$(OPENSSL_CONFIG_TARGET) ; \
	fi
	touch $@

$(OPENSSL_BUILD): prepare $(OPENSSL_CONFIG)
	+if [ ! -f $(OPENSSL_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(OPENSSL_CFLAGS)" \
			-e CXXFLAGS="$(OPENSSL_CFLAGS)" \
			-e LDFLAGS="-static" \
			-e PKG_CONFIG="pkg-config --static" \
			-w $(DOCKER_WORKDIR)/openssl \
			$(BUILDER) \
			/usr/bin/make ; \
	fi
	touch $@

$(OPENSSL_STAGING): prepare $(OPENSSL_BUILD)
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(OPENSSL_CFLAGS)" \
		-e CXXFLAGS="$(OPENSSL_CFLAGS)" \
		-e LDFLAGS="-static" \
		-e PKG_CONFIG="pkg-config --static" \
		-w $(DOCKER_WORKDIR)/openssl \
		$(BUILDER) \
		/usr/bin/make install_sw
	if [ -d "$(TARGET_STAGING)/openssl/lib64" ]; then \
		ln -s lib64 $(TARGET_STAGING)/openssl/lib || true ; \
	fi
	if [ ! -f $(OPENSSL_CACHE_HIT) ]; then \
		rm -rf $(OPENSSL_CACHE_PATH) && \
		mkdir -p $(OPENSSL_CACHE_PATH) && \
		cp -a --reflink=auto $(TARGET_WORKDIR)/openssl $(OPENSSL_CACHE_PATH) ; \
	fi
	touch $@

openssl: $(OPENSSL_STAGING)

# libnetfilter_acct rules

$(LIBNFACCT_DIR): prepare
	rm -rf $(TARGET_WORKDIR)/libnfacct
	if [ -d $(LIBNFACCT_CACHE_PATH)/libnetfilter_acct ]; then \
		cp -a --reflink=auto $(LIBNFACCT_CACHE_PATH)/libnetfilter_acct $(TARGET_WORKDIR) && \
		touch $(LIBNFACCT_CACHE_HIT) ; \
	else \
		rm -f $(LIBNFACCT_CACHE_HIT) && \
		wget --no-verbose -O $(LIBNFACCT_ARCHIVE) $(LIBNFACCT_SOURCE) && \
		echo "$(LIBNFACCT_HASH)  $(LIBNFACCT_ARCHIVE)" | sha256sum -c && \
		mkdir -p $(TARGET_WORKDIR)/libnetfilter_acct && \
		tar -xapf $(LIBNFACCT_ARCHIVE) -C $(TARGET_WORKDIR)/libnetfilter_acct/ --strip-components=1 && \
		rm -f $(LIBNFACCT_ARCHIVE) ; \
	fi
	touch $@

$(LIBNFACCT_CONFIG): prepare $(LIBNFACCT_DIR)
	+if [ ! -f $(LIBNFACCT_CACHE_HIT) ]; then \
		rm -f $(TARGET_WORKDIR)/libnetfilter_acct/libtool && \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(LIBNFACCT_CFLAGS)" \
			-e CXXFLAGS="$(LIBNFACCT_CFLAGS)" \
			-e LDFLAGS="$(LIBNFACCT_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH=/usr/lib/pkgconfig \
			-w $(DOCKER_WORKDIR)/libnetfilter_acct \
			$(BUILDER) \
			/usr/bin/autoreconf -if ; \
	fi
	+if [ ! -f $(LIBNFACCT_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(LIBNFACCT_CFLAGS)" \
			-e CXXFLAGS="$(LIBNFACCT_CFLAGS)" \
			-e LDFLAGS="$(LIBNFACCT_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH=/usr/lib/pkgconfig \
			-w $(DOCKER_WORKDIR)/libnetfilter_acct \
			$(BUILDER) \
			/bin/bash ./configure \
				--prefix=$(LIBNFACCT_DOCKER_STAGING) \
				--exec-prefix=$(LIBNFACCT_DOCKER_STAGING) \
				--disable-dependency-tracking \
				; \
	fi
	touch $@

$(LIBNFACCT_BUILD): prepare $(LIBNFACCT_CONFIG)
	+if [ ! -f $(LIBNFACCT_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(LIBNFACCT_CFLAGS)" \
			-e CXXFLAGS="$(LIBNFACCT_CFLAGS)" \
			-e LDFLAGS="$(LIBNFACCT_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH=/usr/lib/pkgconfig \
			-w $(DOCKER_WORKDIR)/libnetfilter_acct \
			$(BUILDER) \
			/usr/bin/make ; \
	fi
	touch $@

$(LIBNFACCT_STAGING): prepare $(LIBNFACCT_BUILD)
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(LIBNFACCT_CFLAGS)" \
		-e CXXFLAGS="$(LIBNFACCT_CFLAGS)" \
		-e LDFLAGS="$(LIBNFACCT_LDFLAGS)" \
		-e PKG_CONFIG="pkg-config --static" \
		-e PKG_CONFIG_PATH=/usr/lib/pkgconfig \
		-w $(DOCKER_WORKDIR)/libnetfilter_acct \
		$(BUILDER) \
		/usr/bin/make install
	if [ ! -f $(LIBNFACCT_CACHE_HIT) ]; then \
		rm -rf $(LIBNFACCT_CACHE_PATH) && \
		mkdir -p $(LIBNFACCT_CACHE_PATH) && \
		cp -a --reflink=auto $(TARGET_WORKDIR)/libnetfilter_acct $(LIBNFACCT_CACHE_PATH) ; \
	fi
	touch $@

libnetfilter_acct: $(LIBNFACCT_STAGING)

# curl rules

$(CURL_DIR): prepare
	rm -rf $(TARGET_WORKDIR)/curl
	if [ -d $(CURL_CACHE_PATH)/curl ]; then \
		cp -a --reflink=auto $(CURL_CACHE_PATH)/curl $(TARGET_WORKDIR) && \
		touch $(CURL_CACHE_HIT) ; \
	else \
		rm -f $(CURL_CACHE_HIT) && \
		git clone --quiet --branch "$(CURL_VERSION)" --single-branch --depth 1 "$(CURL_SOURCE)" "$(TARGET_WORKDIR)/curl" ; \
	fi
	touch $@

$(CURL_CONFIG): prepare $(CURL_DIR) | $(OPENSSL_STAGING)
	+if [ ! -f $(CURL_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(CURL_CFLAGS)" \
			-e CXXFLAGS="$(CURL_CFLAGS)" \
			-e LDFLAGS="$(CURL_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/curl \
			$(BUILDER) \
			/usr/bin/autoreconf -if ; \
	fi
	+if [ ! -f $(CURL_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(CURL_CFLAGS)" \
			-e CXXFLAGS="$(CURL_CFLAGS)" \
			-e LDFLAGS="$(CURL_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/curl \
			$(BUILDER) \
			/bin/bash ./configure \
				--prefix=$(CURL_DOCKER_STAGING) \
				$(if $(DEVTEST),--disable-optimize,--enable-optimize) \
				--disable-docs \
				--disable-shared \
				--enable-static \
				--enable-http \
				--disable-ldap \
				--disable-ldaps \
				--enable-proxy \
				--disable-dict \
				--disable-telnet \
				--disable-tftp \
				--disable-pop3 \
				--disable-imap \
				--disable-ipfs \
				--disable-rtsp \
				--disable-smb \
				--disable-smtp \
				--disable-gopher \
				--enable-ipv6 \
				--enable-cookies \
				--with-ca-fallback \
				--with-openssl=$(DOCKER_STAGING)/openssl \
				--with-ca-bundle=/opt/netdata/etc/ssl/certs/ca-certificates.crt \
				--with-ca-path=/opt/netdata/etc/ssl/certs \
				--without-brotli \
				--disable-dependency-tracking \
				; \
	fi
	sed -i -e "s/LDFLAGS =/LDFLAGS = -all-static/" $(TARGET_WORKDIR)/curl/src/Makefile
	touch $@

$(CURL_BUILD): prepare $(CURL_CONFIG)
	+if [ ! -f $(CURL_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(CURL_CFLAGS)" \
			-e CXXFLAGS="$(CURL_CFLAGS)" \
			-e LDFLAGS="$(CURL_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/curl \
			$(BUILDER) \
			/usr/bin/make ; \
	fi
	touch $@

$(CURL_STAGING): prepare $(CURL_BUILD)
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(CURL_CFLAGS)" \
		-e CXXFLAGS="$(CURL_CFLAGS)" \
		-e LDFLAGS="$(CURL_LDFLAGS)" \
		-e PKG_CONFIG="pkg-config --static" \
		-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
		-w $(DOCKER_WORKDIR)/curl \
		$(BUILDER) \
		/usr/bin/make install ;
	if [ ! -f $(CURL_CACHE_HIT) ]; then \
		rm -rf $(CURL_CACHE_PATH) && \
		mkdir -p $(CURL_CACHE_PATH) && \
		cp -a --reflink=auto $(TARGET_WORKDIR)/curl $(CURL_CACHE_PATH) ; \
	fi
	touch $@

$(TARGET_INSTALL)/bin/curl: $(CURL_STAGING)
	mkdir -p $(TARGET_INSTALL)/bin
	cp -a --reflink=auto $(TARGET_STAGING)/curl/bin/curl $@

curl: $(TARGET_INSTALL)/bin/curl

# bash rules

$(BASH_DIR): prepare
	rm -rf $(TARGET_WORKDIR)/bash
	if [ -d $(BASH_CACHE_PATH)/bash ]; then \
		cp -a --reflink=auto $(BASH_CACHE_PATH)/bash $(TARGET_WORKDIR) && \
		touch $(BASH_CACHE_HIT) ; \
	else \
		rm -f $(BASH_CACHE_HIT) && \
		wget --no-verbose -O $(BASH_ARCHIVE) $(BASH_SOURCE) && \
		echo "$(BASH_HASH)  $(BASH_ARCHIVE)" | sha256sum -c && \
		mkdir -p $(TARGET_WORKDIR)/bash && \
		tar -xapf $(BASH_ARCHIVE) -C $(TARGET_WORKDIR)/bash/ --strip-components=1 && \
		rm -f $(BASH_ARCHIVE) ; \
	fi
	touch $@

$(BASH_CONFIG): prepare $(BASH_DIR) | $(OPENSSL_STAGING)
	+if [ ! -f $(BASH_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(BASH_CFLAGS)" \
			-e CXXFLAGS="$(BASH_CFLAGS)" \
			-e LDFLAGS="$(BASH_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/bash \
			$(BUILDER) \
			/usr/bin/autoreconf -if ; \
	fi
	+if [ ! -f $(BASH_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(BASH_CFLAGS)" \
			-e CXXFLAGS="$(BASH_CFLAGS)" \
			-e LDFLAGS="$(BASH_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/bash \
			$(BUILDER) \
			/bin/bash ./configure \
				--prefix="$(DOCKER_INSTALL)" \
				--without-bash-malloc \
				--without-afs \
				--enable-static-link \
				--enable-net-redirections \
				--enable-array-variables \
				--disable-progcomp \
				--disable-profiling \
				--disable-nls \
				--disable-dependency-tracking \
				; \
	fi
	touch $@

$(BASH_BUILD): prepare $(BASH_CONFIG)
	+if [ ! -f $(BASH_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(BASH_CFLAGS)" \
			-e CXXFLAGS="$(BASH_CFLAGS)" \
			-e LDFLAGS="$(BASH_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/bash \
			$(BUILDER) \
			/usr/bin/make ; \
	fi
	touch $@

$(TARGET_INSTALL)/bin/bash: prepare $(BASH_BUILD)
	mkdir -p $(@D)
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(BASH_CFLAGS)" \
		-e CXXFLAGS="$(BASH_CFLAGS)" \
		-e LDFLAGS="$(BASH_LDFLAGS)" \
		-e PKG_CONFIG="pkg-config --static" \
		-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
		-w $(DOCKER_WORKDIR)/bash \
		$(BUILDER) \
		/usr/bin/make install
	if [ ! -f $(BASH_CACHE_HIT) ]; then \
		rm -rf $(BASH_CACHE_PATH) && \
		mkdir -p $(BASH_CACHE_PATH) && \
		cp -a --reflink=auto $(TARGET_WORKDIR)/bash $(BASH_CACHE_PATH) ; \
	fi

bash: $(TARGET_INSTALL)/bin/bash

# ioping rules

$(IOPING_DIR): prepare
	rm -rf $(TARGET_WORKDIR)/ioping
	if [ -d $(IOPING_CACHE_PATH)/ioping ]; then \
		cp -a --reflink=auto $(IOPING_CACHE_PATH)/ioping $(TARGET_WORKDIR) && \
		touch $(IOPING_CACHE_HIT) ; \
	else \
		rm -f $(IOPING_CACHE_HIT) && \
		wget --no-verbose -O $(IOPING_ARCHIVE) $(IOPING_SOURCE) && \
		echo "$(IOPING_HASH)  $(IOPING_ARCHIVE)" | sha256sum -c && \
		mkdir -p $(TARGET_WORKDIR)/ioping && \
		tar -xapf $(IOPING_ARCHIVE) -C $(TARGET_WORKDIR)/ioping/ --strip-components=1 && \
		rm -f "$(IOPING_ARCHIVE)" ; \
	fi
	touch $@

$(IOPING_CONFIG): prepare $(IOPING_DIR)
	touch $@

$(IOPING_BUILD): prepare $(IOPING_CONFIG)
	+if [ ! -f $(IOPING_CACHE_HIT) ]; then \
		$(DOCKER) run $(DOCKER_ARGS) \
			$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
			-e CFLAGS="$(IOPING_CFLAGS)" \
			-e CXXFLAGS="$(IOPING_CFLAGS)" \
			-e LDFLAGS="$(IOPING_LDFLAGS)" \
			-e PKG_CONFIG="pkg-config --static" \
			-e PKG_CONFIG_PATH="$(OPENSSL_PKGCONFIG_PATH)" \
			-w $(DOCKER_WORKDIR)/ioping \
			$(BUILDER) \
			/usr/bin/make ; \
	fi
	touch $@

$(TARGET_INSTALL)/usr/libexec/netdata/plugins.d/ioping: prepare $(IOPING_BUILD)
	mkdir -p $(@D)
	$(DOCKER) run $(DOCKER_ARGS) $(BUILDER) \
		/usr/bin/install -o root -g root -m 4750 $(DOCKER_WORKDIR)/ioping/ioping $(DOCKER_INSTALL)/usr/libexec/netdata/plugins.d/
	if [ ! -f $(IOPING_CACHE_HIT) ]; then \
		rm -rf $(IOPING_CACHE_PATH) && \
		mkdir -p $(IOPING_CACHE_PATH) && \
		cp -a --reflink=auto $(TARGET_WORKDIR)/ioping $(IOPING_CACHE_PATH) ; \
	fi

ioping: $(TARGET_INSTALL)/usr/libexec/netdata/plugins.d/ioping

# netdata rules

$(NETDATA_DIR): prepare
	rm -rf $(TARGET_WORKDIR)/netdata
	mkdir -p $(TARGET_WORKDIR)/netdata
	touch $@

$(NETDATA_CONFIG): prepare $(NETDATA_DIR) $(NETDATA_BUILD_DEPS)
	rm -f $@
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(NETDATA_CFLAGS)" \
		-e CXXFLAGS="$(NETDATA_CFLAGS)" \
		-e LDFLAGS="$(NETDATA_LDFLAGS)" \
		-e RUSTFLAGS="$(NETDATA_RUSTFLAGS)" \
		-e PKG_CONFIG="pkg-config --static" \
		-e PKG_CONFIG_PATH="$(NETDATA_PKGCONFIG_PATH)" \
		$(BUILDER) \
		/usr/bin/cmake \
			-S $(NETDATA_SOURCE_DIR) \
			-B $(NETDATA_BUILD_DIR) \
			-G Ninja \
			--fresh \
			$(NETDATA_CMAKE_OPTS) ;
	touch $@

$(NETDATA_BUILD): prepare $(NETDATA_CONFIG)
	rm -f $@
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(NETDATA_CFLAGS)" \
		-e CXXFLAGS="$(NETDATA_CFLAGS)" \
		-e LDFLAGS="$(NETDATA_LDFLAGS)" \
		-e RUSTFLAGS="$(NETDATA_RUSTFLAGS)" \
		-e PKG_CONFIG="pkg-config --static" \
		-e PKG_CONFIG_PATH="$(NETDATA_PKGCONFIG_PATH)" \
		$(BUILDER) \
		/usr/bin/cmake --build $(NETDATA_BUILD_DIR) ;
	touch $@

$(NETDATA_INSTALL): prepare $(NETDATA_BUILD)
	rm -rf $@ $(TARGET_INSTALL)
	mkdir -p $(TARGET_INSTALL) $(TARGET_INSTALL)/bin $(TARGET_INSTALL)/usr $(TARGET_INSTALL)/bin/srv
	+$(DOCKER) run $(DOCKER_ARGS) \
		$(shell $(MAKEFILE_DIR)/scripts/docker-makeflags.sh) \
		-e CFLAGS="$(NETDATA_CFLAGS)" \
		-e CXXFLAGS="$(NETDATA_CFLAGS)" \
		-e LDFLAGS="$(NETDATA_LDFLAGS)" \
		-e RUSTFLAGS="$(NETDATA_RUSTFLAGS)" \
		-e PKG_CONFIG="pkg-config --static" \
		-e PKG_CONFIG_PATH="$(NETDATA_PKGCONFIG_PATH)" \
		$(BUILDER) \
		/bin/sh $(NETDATA_SOURCE_DIR)/packaging/static/scripts/install-staging-tree.sh $(NETDATA_BUILD_DIR) /$(NETDATA_PREFIX) $(DOCKER_INSTALL)
	mv $(TARGET_INSTALL)/bin/netdata $(TARGET_INSTALL)/bin/srv/netdata
	cp -a --reflink=auto $(MAKEFILE_DIR)/wrapper.sh $(TARGET_INSTALL)/bin/netdata
	rm -f $(TARGET_INSTALL)/etc/.install-type
	rm -f $(TARGET_INSTALL)/etc/netdata/netdata.conf
	touch $@

netdata: $(NETDATA_INSTALL)

# Testing rules

check-static-linking: prepare $(NETDATA_INSTALL)
	$(DOCKER) run $(DOCKER_ARGS) \
		$(BUILDER) \
		/bin/sh $(NETDATA_SOURCE_DIR)/packaging/static/scripts/check-static-linking.sh $(DOCKER_INSTALL)

check-systemd-caps: $(NETDATA_INSTALL)
	$(MAKEFILE_DIR)/scripts/systemd-caps-check.sh $(MAKEFILE_DIR) $(TARGET_INSTALL)

runtime-check: prepare $(NETDATA_INSTALL)
	$(DOCKER) run $(DOCKER_ARGS) \
		$(BUILDER) \
		/bin/sh $(NETDATA_SOURCE_DIR)/packaging/static/scripts/runtime-check.sh $(DOCKER_INSTALL) $(NETDATA_SOURCE_DIR) $(BUILDARCH)
	rm -rf $(TARGET_INSTALL)/var/log/netdata $(TARGET_INSTALL)/var/lib/netdata $(TARGET_INSTALL)/var/cache/netdata

netdata-elf-check: $(NETDATA_INSTALL)
	$(MAKEFILE_DIR)/scripts/elf-check.sh $(TARGET_INSTALL)/bin/srv/netdata $(ELF_MACHINE) $(ELF_CLASS)

go-elf-check: $(NETDATA_INSTALL)
	$(MAKEFILE_DIR)/scripts/elf-check.sh $(TARGET_INSTALL)/usr/libexec/netdata/plugins.d/go.d.plugin $(ELF_MACHINE) $(ELF_CLASS)

topology-check: $(NETDATA_INSTALL)
	$(MAKEFILE_DIR)/scripts/check-topology-ip-intel-dir.sh $(TARGET_INSTALL)

cortex-a53-erratum-843419-check: $(NETDATA_INSTALL)
	python3 $(MAKEFILE_DIR)/scripts/check-a53-erratum-843419.py $(TARGET_INSTALL)

TEST_DEPS := check-static-linking \
             runtime-check \
             netdata-elf-check \
             go-elf-check \
             check-systemd-caps \
             $(if $(strip $(TOPOLOGY_STOCK_DIR)),topology-check,) \
             $(if $(filter aarch64,$(BUILDARCH)),cortex-a53-erratum-843419-check,) \
             $(NULL)

tests: $(TEST_DEPS)

# Archive rules

$(TARGET_STAMPS)/ca-certs.stamp: $(NETDATA_INSTALL)
	rm -f $@ $(TARGET_INSTALL)/usr/share/ssl
	$(DOCKER) run $(DOCKER_ARGS) \
		$(BUILDER) \
		cp -a --reflink=auto /etc/ssl $(DOCKER_INSTALL)/usr/share/ssl
	touch $@

$(TARGET_INSTALL)/etc/.install-type: $(NETDATA_INSTALL)
	echo INSTALL_TYPE='manual-static' >> $@
	echo PREBUILT_ARCH='$(BUILDARCH)' >> $@

%/.keep: | runtime-check
	mkdir -p $(@D)
	touch $@

$(TARGET_INSTALL)/system/functions.sh: $(SRCDIR)/packaging/installer/functions.sh
	mkdir -p $(@D)
	cp -a --reflink=auto $< $@

$(TARGET_INSTALL)/system/post-installer.sh: $(SRCDIR)/packaging/static/installer/post-installer.sh
	mkdir -p $(@D)
	cp -a --reflink=auto $< $@

$(TARGET_INSTALL)/system/install-or-update.sh: $(SRCDIR)/packaging/static/installer/install-or-update.sh
	mkdir -p $(@D)
	cp -a --reflink=auto $< $@

makeself.lsm: makeself.lsm.in
	sed -e 's|NETDATA_VERSION|$(VERSION)|' < $< > $@

archives/netdata-$(BUILDARCH)-$(VERSION).gz.run: $(ARCHIVE_DEPS)
	mkdir -p $(@D)
	rm -f $@
	$(MAKEFILE_DIR)/makeself/makeself.sh \
		--gzip --complevel 9 \
		$(MAKESELF_OPTS)

archives/netdata-$(BUILDARCH)-$(VERSION).zstd.run: $(ARCHIVE_DEPS)
	mkdir -p $(@D)
	rm -f $@
	$(MAKEFILE_DIR)/makeself/makeself.sh \
		--zstd --complevel 19 \
		$(MAKESELF_OPTS)

archives/netdata-$(BUILDARCH)-latest.gz.run: archives/netdata-$(BUILDARCH)-$(VERSION).gz.run
	cp -a --reflink=auto $< $@

archives/netdata-$(BUILDARCH)-latest.zstd.run: archives/netdata-$(BUILDARCH)-$(VERSION).zstd.run
	cp -a --reflink=auto $< $@

archive-gz: archives/netdata-$(BUILDARCH)-latest.gz.run
archive-zstd: archives/netdata-$(BUILDARCH)-latest.zstd.run
