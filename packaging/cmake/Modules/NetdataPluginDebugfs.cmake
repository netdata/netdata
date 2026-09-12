# SPDX-License-Identifier: GPL-3.0-or-later
# debugfs: kernel debugfs collectors, with the vendored libsensors subproject.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# add_subdirectory() and CMAKE_CURRENT_BINARY_DIR below are relative to the
# repository and build roots, which include() preserves. This was the only
# block in the root file that used either until src/libnetdata got its own
# CMakeLists.txt, which the root now add_subdirectory()s directly.
#
# The source list lives here rather than in the shared
# NetdataSourceLists.cmake, so a plugin's whole definition is in one place. It
# sits above the guard, so it is defined unconditionally; nothing outside this
# file reads it.

include_guard()

# debugfs.plugin
set(DEBUGFS_PLUGIN_FILES
        src/collectors/debugfs.plugin/debugfs_plugin.c
        src/collectors/debugfs.plugin/debugfs_plugin.h
        src/collectors/debugfs.plugin/module-numa-extfrag.c
        src/collectors/debugfs.plugin/module-zswap.c
        src/collectors/debugfs.plugin/module-devices-powercap.c
        src/collectors/debugfs.plugin/module-libsensors.c
        src/collectors/debugfs.plugin/module-audit.c
)

if(ENABLE_PLUGIN_DEBUGFS)
    # Define debugfs.plugin source files
    # Add executable for debugfs.plugin
    add_executable(debugfs.plugin ${DEBUGFS_PLUGIN_FILES})

    # Add vendored libsensors library
    add_subdirectory(src/collectors/debugfs.plugin/libsensors)

    # Link debugfs.plugin with vendored libsensors
    target_link_libraries(debugfs.plugin PRIVATE vendored_libsensors)

    # Include vendored libsensors headers
    target_include_directories(debugfs.plugin PRIVATE
            src/collectors/debugfs.plugin/libsensors/vendored/lib
            ${CMAKE_CURRENT_BINARY_DIR}/src/collectors/debugfs.plugin/libsensors # For generated headers
    )

    # Link against libnetdata and optionally libcap
    target_link_libraries(debugfs.plugin PRIVATE libnetdata
            "$<$<BOOL:${CAP_FOUND}>:PkgConfig::CAP>")

    # Install the debugfs.plugin binary
    install(TARGETS debugfs.plugin
            COMPONENT plugin-debugfs
            DESTINATION ${PLUGINS_DEST})

    install(FILES
            src/collectors/debugfs.plugin/libsensors/vendored/etc/sensors.conf.default
            COMPONENT ${NETDATA_SENSORS3_COMPONENT}
            DESTINATION ${LIBCONFIG_DEST}
            RENAME sensors3.conf)

    # Install additional packaging files if building for packaging
    netdata_add_deb_copyright(plugin-debugfs netdata-plugin-debugfs)
endif()
