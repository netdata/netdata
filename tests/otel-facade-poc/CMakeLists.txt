# SPDX-License-Identifier: GPL-3.0-or-later

# Keep component selection in the same manifest used by the standalone build.
# OCB emits path: replacements without quoting; use explicit quoted replacements
# here so an out-of-source build also works with spaces in the checkout path.
set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS builder-config.yaml)
file(READ "${CMAKE_CURRENT_SOURCE_DIR}/builder-config.yaml" otelcolpoc_manifest)
string(REPLACE "    path: .\n" "" otelcolpoc_manifest "${otelcolpoc_manifest}")
string(REPLACE "  output_path: .build/collector" "  output_path: collector"
       otelcolpoc_manifest "${otelcolpoc_manifest}")
string(REPLACE "=> ../../../../src/go"
       "=> \"${CMAKE_SOURCE_DIR}/src/go\"\n  - github.com/netdata/netdata/otel-facade-poc => \"${CMAKE_CURRENT_SOURCE_DIR}\""
       otelcolpoc_manifest "${otelcolpoc_manifest}")
string(REPLACE "'" "''" otelcolpoc_go "${GO_EXECUTABLE}")
string(REPLACE "dist:\n" "dist:\n  go: '${otelcolpoc_go}'\n"
       otelcolpoc_manifest "${otelcolpoc_manifest}")
file(GENERATE OUTPUT "${CMAKE_CURRENT_BINARY_DIR}/builder-config.yaml" CONTENT "${otelcolpoc_manifest}")

file(GLOB_RECURSE otelcolpoc_deps CONFIGURE_DEPENDS
     "${CMAKE_CURRENT_SOURCE_DIR}/internal/*.go"
     "${CMAKE_CURRENT_SOURCE_DIR}/internal/*.json"
     "${CMAKE_CURRENT_SOURCE_DIR}/netdataextension/*.go"
     "${CMAKE_CURRENT_SOURCE_DIR}/netdataprovider/*.go"
     "${CMAKE_SOURCE_DIR}/src/go/*.go")
list(APPEND otelcolpoc_deps
     "${CMAKE_CURRENT_SOURCE_DIR}/go.mod" "${CMAKE_CURRENT_SOURCE_DIR}/go.sum"
     "${CMAKE_SOURCE_DIR}/src/go/go.mod" "${CMAKE_SOURCE_DIR}/src/go/go.sum"
     "${CMAKE_CURRENT_BINARY_DIR}/builder-config.yaml")

add_custom_command(
    OUTPUT "${CMAKE_CURRENT_BINARY_DIR}/collector/otel-facade"
    COMMAND "${CMAKE_COMMAND}" -E env "GOROOT=${GO_ROOT}" CGO_ENABLED=0
            GOPROXY=https://proxy.golang.org,direct
            "${GO_EXECUTABLE}" run go.opentelemetry.io/collector/cmd/builder@v0.157.0
            --config "${CMAKE_CURRENT_BINARY_DIR}/builder-config.yaml"
    DEPENDS ${otelcolpoc_deps}
    WORKING_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}"
    COMMENT "Building the experimental OTel Collector DynCfg facade"
    VERBATIM
)
add_custom_target(otelcolpoc ALL DEPENDS "${CMAKE_CURRENT_BINARY_DIR}/collector/otel-facade")

# This opt-in POC ships with the Agent, without adding a production subpackage.
install(PROGRAMS "${CMAKE_CURRENT_BINARY_DIR}/collector/otel-facade"
                 "${CMAKE_CURRENT_SOURCE_DIR}/otel-facade.plugin"
        COMPONENT netdata DESTINATION usr/libexec/netdata/plugins.d)
