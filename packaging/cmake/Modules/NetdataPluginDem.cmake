# SPDX-License-Identifier: GPL-3.0-or-later
# dem.plugin: the experimental digital experience monitoring plugin, its
# synthetic-runner assets and its config tree.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Packaging.cmake registers the plugin-dem component under the same
# ENABLE_PLUGIN_DEM guard; keep the two in step.

include_guard()

if(ENABLE_PLUGIN_DEM)
    if(OS_WINDOWS)
        set(DEM_PLUGIN_BIN dem.plugin.exe)
    else()
        set(DEM_PLUGIN_BIN dem.plugin)
    endif()

    add_go_target(dem-plugin ${DEM_PLUGIN_BIN} src/go cmd/demplugin)
    netdata_add_deb_copyright(plugin-dem netdata-plugin-dem)
    install(PROGRAMS ${CMAKE_BINARY_DIR}/${DEM_PLUGIN_BIN}
            COMPONENT plugin-dem DESTINATION ${PLUGINS_DEST})
    install(FILES
            src/go/plugin/dem/synthetic/runner/assets/run.mjs
            src/go/plugin/dem/synthetic/runner/assets/reporter.cjs
            src/go/plugin/dem/synthetic/runner/assets/protocol.cjs
            src/go/plugin/dem/synthetic/runner/assets/lighthouse.mjs
            src/go/plugin/dem/synthetic/runner/assets/manifest.json
            src/go/plugin/dem/synthetic/runner/assets/package.json
            src/go/plugin/dem/synthetic/runner/assets/package-lock.json
            COMPONENT plugin-dem DESTINATION ${PLUGINS_DEST}/dem)
    install(FILES src/go/plugin/dem/config/dem.conf
            COMPONENT plugin-dem DESTINATION ${LIBCONFIG_DEST})
    install(DIRECTORY src/go/plugin/dem/config/dem
            COMPONENT plugin-dem DESTINATION ${LIBCONFIG_DEST}
            FILES_MATCHING PATTERN "*.conf")
endif()
