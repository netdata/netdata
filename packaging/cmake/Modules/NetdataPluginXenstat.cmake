# SPDX-License-Identifier: GPL-3.0-or-later
# xenstat: Xen domain collection.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together; that is the only form of source-list extraction that does not risk the NETDATA_FILES/LIBNETDATA_FILES composition.

if(ENABLE_PLUGIN_XENSTAT)
    pkg_check_modules(XENSTAT REQUIRED xenstat)
    pkg_check_modules(XENLIGHT REQUIRED xenlight)

    set(XENSTAT_PLUGIN_FILES src/collectors/xenstat.plugin/xenstat_plugin.c)

    add_executable(xenstat.plugin ${XENSTAT_PLUGIN_FILES})
    target_link_libraries (xenstat.plugin libnetdata ${XENLIGHT_LIBRARIES} ${XENSTAT_LIBRARIES})
    target_include_directories(xenstat.plugin PRIVATE ${XENLIGHT_INCLUDE_DIRS} ${XENSTAT_INCLUDE_DIRS})
    target_compile_options(xenstat.plugin PRIVATE ${XENLIGHT_CFLAGS_OTHER} ${XENSTAT_CFLAGS_OTHER})

    install(TARGETS xenstat.plugin
            COMPONENT plugin-xenstat
            DESTINATION ${PLUGINS_DEST})

    netdata_add_deb_copyright(plugin-xenstat netdata-plugin-xenstat)
endif()
