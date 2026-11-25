# SPDX-License-Identifier: GPL-3.0-or-later
# wmi.plugin: the experimental remote Windows WMI plugin and its config tree.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(ENABLE_PLUGIN_WMI)
    if(OS_WINDOWS)
        set(WMI_PLUGIN_BIN wmi.plugin.exe)
    else()
        set(WMI_PLUGIN_BIN wmi.plugin)
    endif()

    add_go_target(wmi-plugin ${WMI_PLUGIN_BIN} src/go cmd/wmiplugin)
    # add_go_target tracks only Go sources; rebuild when the embedded files change too.
    add_custom_command(OUTPUT ${WMI_PLUGIN_BIN} APPEND DEPENDS
                       src/go/plugin/wmi/collector/windows/charts.yaml
                       src/go/plugin/wmi/collector/windows/config_schema.json)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${WMI_PLUGIN_BIN}
            COMPONENT plugin-wmi
            DESTINATION ${PLUGINS_DEST})

    install(FILES src/go/plugin/wmi/config/wmi.conf
            COMPONENT plugin-wmi
            DESTINATION ${LIBCONFIG_DEST})

    install(FILES src/go/plugin/wmi/config/wmi/windows.conf
            COMPONENT plugin-wmi
            DESTINATION ${LIBCONFIG_DEST}/wmi)
endif()
