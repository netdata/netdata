# SPDX-License-Identifier: GPL-3.0-or-later
# The native macOS package: distribution, resources, and the build target

include_guard()

# Define the package-macos target that produces the .pkg from this build
# tree. CPack's productbuild generator is not used - see
# packaging/macos/create-pkg.sh for why; this module configures what the
# driver script consumes.
function(netdata_add_macos_package_target)
        set(pkg_dir "${CMAKE_BINARY_DIR}/macos-pkg")

        # The identifier is the upgrade identity macOS Installer keys on, and
        # it becomes an unchangeable public contract the moment a package
        # ships. PROVISIONAL until release approval; nothing is published
        # from CI while the work iterates.
        set(NETDATA_PKG_IDENTIFIER "cloud.netdata.agent")

        # Installer orders upgrades by comparing versions, so the package
        # version must be monotonic; the generic package version carries the
        # commit hash on nightlies, which does not order. The tweak component
        # is the commit count since the release.
        set(NETDATA_PKG_VERSION "${NETDATA_VERSION_MAJOR}.${NETDATA_VERSION_MINOR}.${NETDATA_VERSION_PATCH}.${NETDATA_VERSION_TWEAK}")

        set(NETDATA_PKG_MINIMUM_OS "${CMAKE_OSX_DEPLOYMENT_TARGET}")

        # Installer.app renders resources by extension and rejects .md
        # outright; hand it plain-text copies.
        configure_file("${CMAKE_SOURCE_DIR}/LICENSE" "${pkg_dir}/resources/License.txt" COPYONLY)
        configure_file("${CMAKE_SOURCE_DIR}/README.md" "${pkg_dir}/resources/ReadMe.txt" COPYONLY)

        configure_file("${CMAKE_SOURCE_DIR}/packaging/macos/distribution.dist.in"
                       "${pkg_dir}/distribution.dist" @ONLY)

        set(pkg_output "${CMAKE_BINARY_DIR}/packages/netdata-${NETDATA_PKG_VERSION}-macos-arm64.pkg")

        add_custom_target(package-macos
                COMMAND "${CMAKE_SOURCE_DIR}/packaging/macos/create-pkg.sh"
                        "${CMAKE_BINARY_DIR}"
                        "${pkg_dir}/distribution.dist"
                        "${pkg_dir}/resources"
                        "${NETDATA_PKG_IDENTIFIER}"
                        "${NETDATA_PKG_VERSION}"
                        "${pkg_output}"
                COMMENT "Building the native macOS package"
                USES_TERMINAL
        )
        add_dependencies(package-macos netdata)
endfunction()
