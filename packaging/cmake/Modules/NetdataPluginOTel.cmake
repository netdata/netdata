# SPDX-License-Identifier: GPL-3.0-or-later
# otel-plugin: installs the Rust OpenTelemetry plugin and its stock receiver config.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.

if(ENABLE_PLUGIN_OTEL)
    corrosion_install(TARGETS otel-plugin
                      PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE
                      RUNTIME DESTINATION ${PLUGINS_DEST}
                      COMPONENT plugin-otel)

    install(FILES src/crates/otel-ingestor/configs/otel.d/v1/metrics/hostmetrics-receiver.yaml
            COMPONENT ${NETDATA_OTEL_CONF_COMPONENT}
            DESTINATION ${LIBCONFIG_DEST}/otel.d/v1/metrics)

    install(DIRECTORY COMPONENT plugin-otel DESTINATION ${CONFIG_DEST}/otel.d/v1/metrics)
endif()
