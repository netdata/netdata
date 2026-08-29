# SPDX-License-Identifier: GPL-3.0-or-later
# otel-plugin: installs the Rust OpenTelemetry plugin and its stock receiver config.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(ENABLE_PLUGIN_OTEL)
    corrosion_install(TARGETS otel-plugin
                      PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE
                      RUNTIME DESTINATION ${PLUGINS_DEST}
                      COMPONENT plugin-otel)

    install(FILES src/crates/otel-ingestor/configs/otel.d/v1/metrics/hostmetrics-receiver.yaml
            COMPONENT ${NETDATA_OTEL_CONF_COMPONENT}
            DESTINATION ${LIBCONFIG_DEST}/otel.d/v1/metrics)

    install(DIRECTORY COMPONENT plugin-otel DESTINATION ${CONFIG_DEST}/otel.d/v1/metrics)

    netdata_add_deb_copyright(plugin-otel netdata-plugin-otel)
endif()
