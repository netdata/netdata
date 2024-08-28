# CMake module to handle fetching and installing the dashboard code
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(NetdataUtil)

# Bundle the dashboard code for inclusion during install.
#
# This is unfortunately complicated due to how we need to handle the
# generation of the CMakeLists file for the dashboard code.
function(bundle_dashboard)
  include(ExternalProject)

  set(DASHBOARD_SRC_DIR "${CMAKE_BINARY_DIR}/dashboard-src")
  set(DASHBOARD_BIN_DIR "${CMAKE_BINARY_DIR}/dashboard-bin")
  set(DASHBOARD_URL "https://app.netdata.cloud/agent.tar.gz")

  message(STATUS "Preparing dashboard code")

  message(STATUS "  Fetching ${DASHBOARD_URL}")
  file(DOWNLOAD
       "${DASHBOARD_URL}"
       "${CMAKE_BINARY_DIR}/dashboard.tar"
       TIMEOUT 180
       STATUS fetch_status)

  list(GET fetch_status 0 result)

  if(result)
    message(FATAL_ERROR "Failed to fetch dashboard code")
  else()
    message(STATUS "  Fetching ${DASHBOARD_URL} -- Done")
  endif()

  message(STATUS "  Extracting dashboard code")
  extract_tarball(
    "${CMAKE_BINARY_DIR}/dashboard.tar"
    "${DASHBOARD_SRC_DIR}"
  )
  message(STATUS "  Extracting dashboard code -- Done")

  message(STATUS "  Generating CMakeLists.txt file for dashboard code")
  set(cmakelists "")

  subdirlist(dash_dirs "${DASHBOARD_SRC_DIR}/dist/agent")

  foreach(dir IN LISTS dash_dirs)
    file(GLOB files
         LIST_DIRECTORIES FALSE
         RELATIVE ${DASHBOARD_SRC_DIR}
         "${DASHBOARD_SRC_DIR}/dist/agent/${dir}")

    set(cmakelists "${cmakelists}install(FILES ${files} COMPONENT dashboard DESTINATION ${WEB_DEST}/${dir})\n")
  endforeach()

  file(WRITE "${DASHBOARD_SRC_DIR}/CMakeLists.txt" "${cmakelists}")
  message(STATUS "  Generating CMakeLists.txt file for dashboard code -- Done")
  add_subdirectory("${DASHBOARD_SRC_DIR}" "${DASHBOARD_BIN_DIR}")
  message(STATUS "Preparing dashboard code -- Done")
endfunction()
