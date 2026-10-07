# SPDX-License-Identifier: GPL-3.0-or-later

# Share the standalone manifest, with quoted replacements for checkout paths
# containing spaces and an output directory inside the CMake build tree.
set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS builder-config.yaml)
file(READ "${CMAKE_CURRENT_SOURCE_DIR}/builder-config.yaml" otelcolpoc_manifest)
string(REPLACE "    path: .\n" "" otelcolpoc_manifest "${otelcolpoc_manifest}")
string(REPLACE "  output_path: .build/collector" "  output_path: collector"
       otelcolpoc_manifest "${otelcolpoc_manifest}")
string(APPEND otelcolpoc_manifest
       "\nreplaces:\n  - github.com/netdata/netdata/otel-orchestrator-poc => \"${CMAKE_CURRENT_SOURCE_DIR}\"\n")
string(REPLACE "'" "''" otelcolpoc_go "${GO_EXECUTABLE}")
string(REPLACE "dist:\n" "dist:\n  go: '${otelcolpoc_go}'\n"
       otelcolpoc_manifest "${otelcolpoc_manifest}")
file(GENERATE OUTPUT "${CMAKE_CURRENT_BINARY_DIR}/builder-config.yaml" CONTENT "${otelcolpoc_manifest}")

file(GLOB_RECURSE otelcolpoc_deps CONFIGURE_DEPENDS
     "${CMAKE_CURRENT_SOURCE_DIR}/workerprovider/*.go"
     "${CMAKE_CURRENT_SOURCE_DIR}/workerextension/*.go")
list(APPEND otelcolpoc_deps
     "${CMAKE_CURRENT_SOURCE_DIR}/go.mod" "${CMAKE_CURRENT_SOURCE_DIR}/go.sum"
     "${CMAKE_CURRENT_BINARY_DIR}/builder-config.yaml")

add_custom_command(
    OUTPUT "${CMAKE_CURRENT_BINARY_DIR}/collector/otel-worker"
    COMMAND "${CMAKE_COMMAND}" -E env "GOROOT=${GO_ROOT}" CGO_ENABLED=0
            GOPROXY=https://proxy.golang.org,direct
            "${GO_EXECUTABLE}" run go.opentelemetry.io/collector/cmd/builder@v0.157.0
            --config "${CMAKE_CURRENT_BINARY_DIR}/builder-config.yaml"
    DEPENDS ${otelcolpoc_deps}
    WORKING_DIRECTORY "${CMAKE_CURRENT_BINARY_DIR}"
    COMMENT "Building the experimental OTel Collector worker"
    VERBATIM
)
add_custom_target(otelcolpoc ALL DEPENDS "${CMAKE_CURRENT_BINARY_DIR}/collector/otel-worker")

# Only the manager uses .plugin; Agent discovery must never launch the worker.
install(PROGRAMS "${CMAKE_CURRENT_BINARY_DIR}/collector/otel-worker"
        COMPONENT netdata DESTINATION usr/libexec/netdata/plugins.d)
