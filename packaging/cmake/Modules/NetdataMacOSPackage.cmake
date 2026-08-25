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

        # The LaunchDaemon ships in the payload itself so the receipt tracks
        # it. The destination is absolute on purpose - it is host-owned, not
        # part of the /opt/netdata tree.
        configure_file("${CMAKE_SOURCE_DIR}/packaging/macos/netdata.plist.in"
                       "${pkg_dir}/com.github.netdata.plist" @ONLY)
        install(FILES "${pkg_dir}/com.github.netdata.plist"
                COMPONENT netdata
                DESTINATION /Library/LaunchDaemons)

        # The uninstaller ships inside the payload so every installed Agent
        # carries the exact removal logic matching its own layout. It is
        # operator-run (sudo), unlike the maintainer scripts below.
        configure_file("${CMAKE_SOURCE_DIR}/packaging/macos/netdata-uninstaller.sh.in"
                       "${pkg_dir}/netdata-uninstaller.sh" @ONLY)
        install(PROGRAMS "${pkg_dir}/netdata-uninstaller.sh"
                COMPONENT netdata
                DESTINATION "${LIBEXEC_DEST}")

        # Maintainer scripts: strictly non-interactive, idempotent; pkgbuild
        # requires them executable.
        foreach(script preinstall postinstall)
                configure_file("${CMAKE_SOURCE_DIR}/packaging/macos/scripts/${script}.in"
                               "${pkg_dir}/scripts/${script}" @ONLY)
                file(CHMOD "${pkg_dir}/scripts/${script}"
                     PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE
                                 GROUP_READ GROUP_EXECUTE
                                 WORLD_READ WORLD_EXECUTE)
        endforeach()

        set(pkg_output "${CMAKE_BINARY_DIR}/packages/netdata-${NETDATA_PKG_VERSION}-macos-arm64.pkg")

        add_custom_target(package-macos
                COMMAND "${CMAKE_SOURCE_DIR}/packaging/macos/create-pkg.sh"
                        "${CMAKE_BINARY_DIR}"
                        "${pkg_dir}/distribution.dist"
                        "${pkg_dir}/resources"
                        "${pkg_dir}/scripts"
                        "${NETDATA_PKG_IDENTIFIER}"
                        "${NETDATA_PKG_VERSION}"
                        "${pkg_output}"
                COMMENT "Building the native macOS package"
                USES_TERMINAL
        )
        add_dependencies(package-macos netdata)
endfunction()
