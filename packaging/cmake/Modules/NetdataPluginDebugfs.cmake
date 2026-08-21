# SPDX-License-Identifier: GPL-3.0-or-later
# debugfs: kernel debugfs collectors, with the vendored libsensors subproject.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# add_subdirectory() and CMAKE_CURRENT_BINARY_DIR below are relative to the
# repository and build roots, which include() preserves. This is the only
# block in the root file that used either.
#
# Its source list moved here from NetdataSourceLists.cmake per D27: a plugin
# module owns its own inventory. It stays above the guard so it is still
# defined unconditionally, exactly as before.

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
    target_link_libraries(debugfs.plugin PRIVATE libnetdata ${CAP_LIBRARIES})
    target_include_directories(debugfs.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(debugfs.plugin PRIVATE ${CAP_CFLAGS_OTHER})

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
