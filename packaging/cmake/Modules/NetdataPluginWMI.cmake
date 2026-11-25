# SPDX-License-Identifier: GPL-3.0-or-later
# Source-install opt-in for the remote WMI collector.
include_guard()

if(ENABLE_PLUGIN_WMI)
    if(OS_WINDOWS)
        set(WMI_PLUGIN_BIN wmi.plugin.exe)
    else()
        set(WMI_PLUGIN_BIN wmi.plugin)
    endif()
    add_go_target(wmi-plugin ${WMI_PLUGIN_BIN} src/go cmd/wmiplugin)
    add_custom_command(OUTPUT ${WMI_PLUGIN_BIN} APPEND DEPENDS
                       src/go/plugin/wmi/collector/windows/charts.yaml
                       src/go/plugin/wmi/collector/windows/config_schema.json)

    install(PROGRAMS ${CMAKE_BINARY_DIR}/${WMI_PLUGIN_BIN}
            COMPONENT plugin-wmi DESTINATION ${PLUGINS_DEST})
    install(FILES src/go/plugin/wmi/config/wmi.conf
            COMPONENT plugin-wmi DESTINATION ${LIBCONFIG_DEST})
    install(FILES src/go/plugin/wmi/config/wmi/windows.conf
            COMPONENT plugin-wmi DESTINATION ${LIBCONFIG_DEST}/wmi)
endif()
