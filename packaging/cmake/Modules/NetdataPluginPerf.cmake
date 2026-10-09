# SPDX-License-Identifier: GPL-3.0-or-later
# perf: hardware performance counter collection.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

if(ENABLE_PLUGIN_PERF)
    set(PERF_PLUGIN_FILES src/collectors/perf.plugin/perf_plugin.c)

    add_executable(perf.plugin ${PERF_PLUGIN_FILES})
    target_link_libraries(perf.plugin libnetdata)

    install(TARGETS perf.plugin
            COMPONENT plugin-perf
            DESTINATION ${PLUGINS_DEST})

    netdata_add_deb_copyright(plugin-perf netdata-plugin-perf)
endif()
