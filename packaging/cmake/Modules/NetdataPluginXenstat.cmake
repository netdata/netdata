# SPDX-License-Identifier: GPL-3.0-or-later
# xenstat: Xen domain collection.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

if(ENABLE_PLUGIN_XENSTAT)
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
