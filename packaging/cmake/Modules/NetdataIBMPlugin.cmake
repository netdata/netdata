# SPDX-License-Identifier: GPL-3.0-or-later
# Build configuration for ibm.d.plugin which requires CGO

include(FetchContent)

# add_ibm_plugin_target: Add target for building ibm.d.plugin with CGO
#
# This plugin requires CGO and IBM DB2/MQ headers for compilation.
# The plugin will download IBM libraries on first run.
macro(add_ibm_plugin_target)
  set(IBM_MQ_DIR "${CMAKE_BINARY_DIR}/ibm-mqclient")

  FetchContent_Declare(
    ibm_mq
    SOURCE_DIR "${IBM_MQ_DIR}"
    CONFIGURE_COMMAND "${CMAKE_COMMAND} -E true"
    BUILD_COMMAND "${CMAKE_COMMAND} -E true"
    INSTALL_COMMAND "${CMAKE_COMMAND} -E true"
    URL https://public.dhe.ibm.com/ibmdl/export/pub/software/websphere/messaging/mqdev/redist/9.4.1.0-IBM-MQC-Redist-LinuxX64.tar.gz
    URL_HASH SHA256=f0759f621c3ccbbad2764204fc3d9ccef6409fd8293e23b351cceb3c6c6a0478
  )

  message(STATUS "Fetching IBM MQ client libraries for build")
  FetchContent_MakeAvailable(ibm_mq)

  # Find all Go source files - resolve symlinks for consistent dependency tracking
  get_filename_component(RESOLVED_SOURCE_DIR "${CMAKE_SOURCE_DIR}" REALPATH)
  file(GLOB_RECURSE IBM_PLUGIN_DEPS CONFIGURE_DEPENDS "${RESOLVED_SOURCE_DIR}/src/go/*.go")
  list(APPEND IBM_PLUGIN_DEPS
    "${RESOLVED_SOURCE_DIR}/src/go/go.mod"
    "${RESOLVED_SOURCE_DIR}/src/go/go.sum"
  )

  # Configure build flags for hybrid approach:
  # - Database collectors (AS/400, DB2) use unixODBC exclusively (no IBM headers needed)
  # - MQ collector requires IBM MQ headers and libraries
  set(IBM_CGO_CFLAGS "")
  set(IBM_CGO_LDFLAGS "")
  set(IBM_RPATH_FLAGS "")

  # Add MQ support if MQ client libraries are available
  if(EXISTS "${IBM_MQ_DIR}/inc/cmqc.h")
    set(IBM_CGO_CFLAGS "-I${IBM_MQ_DIR}/inc")
    set(IBM_CGO_LDFLAGS "-L${IBM_MQ_DIR}/lib64 -lmqic_r")
    set(IBM_RPATH_FLAGS "-Wl,-rpath,${IBM_MQ_DIR}/lib64")
    set(MQ_INSTALLATION_PATH "${IBM_MQ_DIR}")
  else()
    message(WARNING "IBM MQ client libraries not found - MQ PCF collector will be disabled")
    set(MQ_INSTALLATION_PATH "")
  endif()

  # Build with CGO enabled and multiple rpath entries for different installation methods
  add_custom_command(
    OUTPUT ibm.d.plugin
    COMMAND "${CMAKE_COMMAND}" -E env
      GOROOT=${GO_ROOT}
      CGO_ENABLED=1
      CGO_CFLAGS=${IBM_CGO_CFLAGS}
      CGO_LDFLAGS=${IBM_CGO_LDFLAGS}
      LD_LIBRARY_PATH=${IBM_MQ_DIR}/lib64:$LD_LIBRARY_PATH
      MQ_INSTALLATION_PATH=${MQ_INSTALLATION_PATH}
      GOPROXY=https://proxy.golang.org,direct
      "${GO_EXECUTABLE}" build
        -buildvcs=false
        -tags "ibm_mq"
        -ldflags "${GO_LDFLAGS} -extldflags '${IBM_RPATH_FLAGS}'"
        -o "${CMAKE_BINARY_DIR}/ibm.d.plugin" "./cmd/ibmdplugin"
    DEPENDS ${IBM_PLUGIN_DEPS}
    COMMENT "Building ibm.d.plugin (with CGO)"
    WORKING_DIRECTORY "${RESOLVED_SOURCE_DIR}/src/go"
    VERBATIM
  )

  add_custom_target(
    ibm-plugin ALL
    DEPENDS ibm.d.plugin
  )
endmacro()
