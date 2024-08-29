# CMake module to handle fetching and installing the dashboard code
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

include(NetdataUtil)

# Add an install rule to the variable for the given source and target
function(_nd_add_dashboard_install_rule var src target)
  set(rule "install(install(FILES ${src} COMPONENT dashboard DESTINATION ${WEB_DEST}/${target})\n")
  set(var "${var}${rule}" PARENT_SCOPE)
endfunction()

# Bundle the dashboard code for inclusion during install.
#
# This is unfortunately complicated due to how we need to handle the
# generation of the CMakeLists file for the dashboard code.
function(bundle_dashboard)
  include(ExternalProject)

  set(dashboard_src_dir "${CMAKE_BINARY_DIR}/dashboard-src")
  set(dashboard_src_prefix "${dashboard_src_dir}/dist/agent")
  set(dashboard_bin_dir "${CMAKE_BINARY_DIR}/dashboard-bin")
  set(DASHBOARD_URL "https://app.netdata.cloud/agent.tar.gz" CACHE STRING
      "URL used to fetch the local agent dashboard code")

  message(STATUS "Preparing dashboard code")

  message(STATUS "  Fetching ${DASHBOARD_URL}")
  file(DOWNLOAD
       "${DASHBOARD_URL}"
       "${CMAKE_BINARY_DIR}/dashboard.tar.gz"
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
    "${CMAKE_BINARY_DIR}/dashboard.tar.gz"
    "${dashboard_src_dir}"
  )
  message(STATUS "  Extracting dashboard code -- Done")

  message(STATUS "  Generating CMakeLists.txt file for dashboard code")
  set(cmakelists "")

  subdirlist(dash_dirs "${dashboard_src_prefix}")

  foreach(dir IN LISTS dash_dirs)
    file(GLOB files
         LIST_DIRECTORIES FALSE
         RELATIVE "${dashboard_src_dir}"
         "${dashboard_src_prefix}/${dir}")

    _nd_add_dashboard_install_rule(cmakelists "${files}" "${dir}")
  endforeach()

  file(GLOB files
       LIST_DIRECTORIES FALSE
       RELATIVE "${dashboard_src_dir}"
       "${dashboard_src_prefix}"

  _nd_add_dashboard_install_rule(cmakelists "${files}" "")

  file(WRITE "${dashboard_src_dir}/CMakeLists.txt" "${cmakelists}")
  message(STATUS "  Generating CMakeLists.txt file for dashboard code -- Done")
  add_subdirectory("${dashboard_src_dir}" "${dashboard_bin_dir}")
  message(STATUS "Preparing dashboard code -- Done")
endfunction()
