# SPDX-License-Identifier: GPL-3.0-or-later
# Functions and macros for handling of OpenSSL

include_guard()

# Handle bundling of OpenSSL for the macOS package.
#
# OpenSSL has no CMake build, so unlike the other bundled libraries it is
# driven as an ExternalProject through its own Perl Configure, producing
# static libssl.a/libcrypto.a. Two choices here are deliberate:
#
#   - --openssldir=/etc/ssl: the compiled-in default lookup then finds
#     Apple's OS-maintained /etc/ssl/cert.pem on a clean machine, so TLS
#     works with zero Agent code change. Trust added through Keychain
#     Access is NOT picked up; enterprises with internal CAs use
#     SSL_CERT_FILE or an appended bundle, as on Linux static installs.
#   - -mmacosx-version-min: an ExternalProject does not inherit
#     CMAKE_OSX_DEPLOYMENT_TARGET the way a FetchContent subdirectory
#     does, so the floor is passed explicitly and the artifact gate
#     verifies it on the produced binaries.
function(netdata_bundle_openssl)
        include(ExternalProject)
        include(ProcessorCount)

        message(STATUS "Preparing bundled OpenSSL")

        set(repo https://github.com/openssl/openssl)
        set(tag 7b371d80d959ec9ab4139d09d78e83c090de9779) # openssl-3.6.0, the pin the static builder uses

        set(install_dir "${CMAKE_BINARY_DIR}/_deps/openssl-install")

        set(configure_args
                darwin64-arm64-cc
                no-shared
                no-module
                no-tests
                no-apps
                threads
                --prefix=${install_dir}
                --libdir=lib
                --openssldir=/etc/ssl
        )

        if(CMAKE_OSX_DEPLOYMENT_TARGET)
                list(APPEND configure_args "-mmacosx-version-min=${CMAKE_OSX_DEPLOYMENT_TARGET}")
        endif()

        ProcessorCount(ncpu)
        if(ncpu EQUAL 0)
                set(ncpu 2)
        endif()

        ExternalProject_Add(bundled-openssl
                GIT_REPOSITORY ${repo}
                GIT_TAG ${tag}
                CONFIGURE_COMMAND perl <SOURCE_DIR>/Configure ${configure_args}
                BUILD_COMMAND make -j${ncpu} build_libs
                INSTALL_COMMAND make install_sw
                INSTALL_DIR "${install_dir}"
                BUILD_BYPRODUCTS
                        "${install_dir}/lib/libssl.a"
                        "${install_dir}/lib/libcrypto.a"
                EXCLUDE_FROM_ALL TRUE
        )

        # The include directory must exist when a consumer's usage
        # requirements are evaluated, which is before the ExternalProject
        # has run.
        file(MAKE_DIRECTORY "${install_dir}/include")

        # Consumers link PkgConfig::TLS and PkgConfig::CRYPTO, the imported
        # targets pkg_check_modules creates on every other build; provide
        # the same names so no consumer changes. Imported targets cannot
        # carry add_dependencies, so an interface target rides along in the
        # link interface to order the ExternalProject before any consumer.
        add_library(netdata-bundled-openssl-order INTERFACE)
        add_dependencies(netdata-bundled-openssl-order bundled-openssl)

        add_library(PkgConfig::CRYPTO STATIC IMPORTED GLOBAL)
        set_target_properties(PkgConfig::CRYPTO PROPERTIES
                IMPORTED_LOCATION "${install_dir}/lib/libcrypto.a"
                INTERFACE_INCLUDE_DIRECTORIES "${install_dir}/include"
                INTERFACE_LINK_LIBRARIES "netdata-bundled-openssl-order")

        add_library(PkgConfig::TLS STATIC IMPORTED GLOBAL)
        set_target_properties(PkgConfig::TLS PROPERTIES
                IMPORTED_LOCATION "${install_dir}/lib/libssl.a"
                INTERFACE_INCLUDE_DIRECTORIES "${install_dir}/include"
                INTERFACE_LINK_LIBRARIES "PkgConfig::CRYPTO;netdata-bundled-openssl-order")

        message(STATUS "Finished preparing bundled OpenSSL")
endfunction()

# Handle setup of OpenSSL for the build.
#
# The pkg package kind must not link libraries from the build host, so it
# always bundles; everything else finds the system copy with pkg-config.
# Either way the result is the PkgConfig::TLS and PkgConfig::CRYPTO targets
# the rest of the build links, and both are required.
macro(netdata_detect_openssl)
        if(NETDATA_PACKAGE_KIND STREQUAL "pkg")
                netdata_bundle_openssl()
        else()
                pkg_check_modules(TLS IMPORTED_TARGET openssl)
                pkg_check_modules(CRYPTO IMPORTED_TARGET libcrypto)
        endif()

        if(NOT TARGET PkgConfig::TLS)
                message(FATAL_ERROR "OpenSSL (or LibreSSL) is required for building Netdata, but could not be found.")
        endif()

        if(NOT TARGET PkgConfig::CRYPTO)
                message(FATAL_ERROR "libcrypto is required for building Netdata, but could not be found.")
        endif()
endmacro()
