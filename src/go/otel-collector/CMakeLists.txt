# SPDX-License-Identifier: GPL-3.0-or-later

function(_handle_otel)
  if(CMAKE_BUILD_TYPE STREQUAL Debug)
    set(DEBUG_BUILD True)
  else()
    set(DEBUG_BUILD False)
  endif()

  message(STATUS "Generating OpenTelemetry Collector Builder configuration")
  configure_file("${CMAKE_CURRENT_SOURCE_DIR}/release-config.yaml.in"
                "${CMAKE_BINARY_DIR}/otel-build-config.yaml"
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
    OUTPUT otel-collector/otelcol.plugin
    COMMAND ${CMAKE_BINARY_DIR}/bin/builder --config=${CMAKE_BINARY_DIR}/otel-build-config.yaml
    DEPENDS ${otelcol_deps}
    COMMENT "Building otelcol.plugin"
    WORKING_DIRECTORY "${CMAKE_CURRENT_SOURCE_DIR}"
    VERBATIM
  )

  add_custom_target(
    plugin-otelcol ALL
    DEPENDS otel-collector/otelcol.plugin
  )
endfunction()

_handle_otel()
