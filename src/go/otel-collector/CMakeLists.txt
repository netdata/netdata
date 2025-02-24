# SPDX-License-Identifier: GPL-3.0-or-later

cmake_minimum_required(VERSION 3.16.0...3.30)

list(APPEND CMAKE_MODULE_PATH "${CMAKE_CURRENT_SOURCE_DIR}/../../../packaging/cmake/Modules")

if(NOT DEFINED NETDATA_VERSION_STRING)
  set(NETDATA_VERSION_STRING v0.0.0)
endif()

foreach(v IN ITEMS MAJOR MINOR PATCH TWEAK)
  if(NOT DEFINED NETDATA_VERSION_${v})
    set(NETDATA_VERSION_${v} 0)
  endif()
endforeach()

project(netdata-otel-collector
        VERSION "${NETDATA_VERSION_MAJOR}.${NETDATA_VERSION_MINOR}.${NETDATA_VERSION_PATCH}.${NETDATA_VERSION_TWEAK}"
        HOMEPAGE_URL "https://www.netdata.cloud"
        LANGUAGES NONE)

if(CMAKE_BUILD_TYPE STREQUAL Debug)
  set(DEBUG_BUILD True)
else()
  set(DEBUG_BUILD False)
endif()

include(NetdataGoTools)

find_min_go_version("${CMAKE_CURRENT_SOURCE_DIR}")

find_package(Go "${MIN_GO_VERSION}" REQUIRED)

message(STATUS "Generating OpenTelemetry Collector Builder configuration")
configure_file("${CMAKE_CURRENT_SOURCE_DIR}/release-config.yaml.in"
               "${CMAKE_BINARY_DIR}/otel-config.yaml"
               @ONLY)
message(STATUS "Generating OpenTelemetry Collector Builder configuration -- Done")

message(STATUS "Fetching OpenTelemetry Collector Builder")
set(OLD_GOBIN $ENV{GOBIN})
set(ENV{GOBIN} ${CMAKE_BINARY_DIR}/bin)
execute_process(
  COMMAND ${GO_EXECUTABLE} install go.opentelemetry.io/collector/cmd/builder@latest
  RESULT_VARIABLE otel_builder_install
)
set(ENV{GOBIN} ${OLD_GOBIN})

if(otel_builder_install)
  message(FATAL_ERROR "Fetching OpenTelemetry Collector Builder --Failed")
else()
  message(STATUS "Fetching OpenTelemetry Collector Builder -- Success")
endif()

set(DIRS "exporter/journaldexporter")
set(otelcol_deps "")

foreach(dir IN LISTS DIRS)
  file(GLOB_RECURSE deps CONFIGURE_DEPENDS "${CMAKE_CURRENT_SOURCE_DIR}/${dir}/*.go")
  list(APPEND otelcol_deps "${deps}")
  list(APPEND otelcol_deps
    "${CMAKE_CURRENT_SOURCE_DIR}/${dir}/go.mod"
    "${CMAKE_CURRENT_SOURCE_DIR}/${dir}/go.sum"
  )
endforeach()

add_custom_command(
  OUTPUT otelcol-plugin/otelcol.plugin
  COMMAND ${CMAKE_BINARY_DIR}/bin/builder --config=${CMAKE_BINARY_DIR}/otel-config.yaml
  DEPENDS ${otelcol_deps}
  COMMENT "Building otelcol.plugin"
  WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}"
  VERBATIM
)
add_custom_target(
  plugin-otelcol ALL
  DEPENDS otelcol-plugin/otelcol.plugin
)
