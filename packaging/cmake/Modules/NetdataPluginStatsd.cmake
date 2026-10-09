# SPDX-License-Identifier: GPL-3.0-or-later
# statsd.plugin: the experimental Go StatsD plugin and its config tree.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(ENABLE_PLUGIN_STATSD)
    if (OS_WINDOWS)
        set(STATSD_GO_PLUGIN_BIN statsd.plugin.exe)
    else()
        set(STATSD_GO_PLUGIN_BIN statsd.plugin)
    endif()

    add_go_target(statsd-go-plugin ${STATSD_GO_PLUGIN_BIN} src/go cmd/statsdplugin)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${STATSD_GO_PLUGIN_BIN}
            COMPONENT plugin-statsd
            DESTINATION ${PLUGINS_DEST})

    install(FILES src/go/plugin/statsd/config/statsd.conf
            COMPONENT plugin-statsd
            DESTINATION ${LIBCONFIG_DEST})

    install(DIRECTORY src/go/plugin/statsd/config/statsd
            COMPONENT plugin-statsd
            DESTINATION ${LIBCONFIG_DEST}
            FILES_MATCHING PATTERN "*.conf")
endif()
