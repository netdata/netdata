# SPDX-License-Identifier: GPL-3.0-or-later

cmake_minimum_required(VERSION 3.16.0...3.30)

list(APPEND RHEL_DISTROS centos centos-stream rocky almalinux cloudlinux)
list(APPEND SUSE_DISTROS opensuse-leap opensuse-tumbleweed)
list(APPEND RPM_DISTROS rhel opensuse ol amzn fedora)
list(APPEND DEB_DISTROS debian ubuntu)

set(DEB_GPG_KEY_SOURCE "https://repo.netdata.cloud/netdatabot.gpg.key")

set(PACKAGE_VERSION 4)
set(PACKAGE_RELEASE 1)

set(CPACK_THREADS 0)
set(CPACK_STRIP_FILES NO)
set(CPACK_PACKAGE_INSTALL_DIRECTORY "netdata")
set(CPACK_PACKAGE_DIRECTORY "${CMAKE_BINARY_DIR}/packages")
set(CPACK_PACKAGING_INSTALL_PREFIX "/")
set(CPACK_PACKAGE_VENDOR "Netdata Inc.")
set(CPACK_COMPONENT_NETDATA-REPO_PACKAGE_DESCRIPTION "Configuration for the official Netdata Stable package repository.")
set(CPACK_COMPONENT_NETDATA-REPO-EDGE_PACKAGE_DESCRIPTION "Configuration for the official Netdata Edge package repository.")

project(netdata-repoconfig VERSION "${PACKAGE_VERSION}.${PACKAGE_RELEASE}"
        DESCRIPTION "Repository configuration for Netdata’s official native packages."
        HOMEPAGE_URL "https://www.netdata.cloud/"
        LANGUAGES NONE)

function(extract_release_item _variable _item)
  if(DEFINED "${_variable}")
    return()
  endif()

  if(DEFINED OS_RELEASE_FILE)
  else()
    message(CHECK_START "Searching for os-release file")
    find_file(OS_RELEASE_FILE os-release PATHS /etc /lib /usr/lib NO_DEFAULT_PATH)
    if(${OS_RELEASE_FILE} STREQUAL "OS_RELEASE_FILE-NOTFOUND")
      message(CHECK_FAIL "failed")
      message(FATAL_ERROR "Could not find os-release file")
    endif()

    message(CHECK_PASS "${OS_RELEASE_FILE}")
  endif()

  message(CHECK_START "Extracting ${_item} from ${OS_RELEASE_FILE}")
  execute_process(COMMAND sh -c ". ${OS_RELEASE_FILE} && printf %s $${_item}"
                  RESULT_VARIABLE _result
                  OUTPUT_VARIABLE _output)

  if(NOT ${_result} EQUAL 0)
    message(CHECK_FAIL "failed to parse ${OS_RELEASE_FILE}")
    return()
  elseif(${_output} STREQUAL "")
    message(CHECK_FAIL "variable ${_item} not defined in ${OS_RELEASE_FILE}")
    return()
  endif()

  message(CHECK_PASS ${_output})
  set(${_variable} ${_output} PARENT_SCOPE)
endfunction()

function(require_command _variable _cmd)
  if(DEFINED ${_variable})
    return()
  endif()

  message(CHECK_START "Looking for ${_cmd}")

  find_program(_result_${_cmd} ${_cmd})

  if(${_result_${_cmd}} STREQUAL "_result_${_cmd}-NOTFOUND")
    message(CHECK_FAIL "failed")
    message(FATAL_ERROR "Unable to find required command: ${_cmd}")
  endif()

  message(CHECK_PASS "${_result_${_cmd}}")
  set(${_variable} ${_result_${_cmd}} PARENT_SCOPE)
endfunction()

if(${CMAKE_SYSTEM_NAME} STREQUAL "Linux")
  extract_release_item("DISTRO" ID)

  if(NOT DEFINED DISTRO)
    message(FATAL_ERROR "Failed to auto-detect distro ID")
  endif()

  extract_release_item(DISTRO_VERSION VERSION_ID)

  if(NOT DEFINED DISTRO_VERSION)
    message(FATAL_ERROR "Failed to auto-detect distro version ID.")
  endif()
else()
  message(FATAL_ERROR "Repository configuration packages can only be built on Linux")
endif()

if(${DISTRO} IN_LIST RHEL_DISTROS)
  set(DISTRO "rhel")
elseif(${DISTRO} STREQUAL "opensuse-leap")
  set(DISTRO "opensuse")
elseif(${DISTRO} STREQUAL "opensuse-tumbleweed")
  set(DISTRO "opensuse")
  set(DISTRO_VERSION "tumbleweed")
endif()

if(${DISTRO} IN_LIST DEB_DISTROS)
  extract_release_item(SUITE VERSION_CODENAME)

  if(NOT DEFINED SUITE)
    message(FATAL_ERROR "Failed to determine version codename")
  endif()

  require_command(DPKG dpkg)
  require_command(CURL curl)
  require_command(GPG gpg)

  set(DIST_NAME ${DISTRO})
  message(STATUS "Generating stable repository configuration for ${DISTRO} ${SUITE}")
  set(VARIANT stable)
  configure_file(netdata.sources.in netdata.sources @ONLY)
  message(STATUS "Generating edge repository configuration for ${DISTRO} ${SUITE}")
  set(VARIANT edge)
  configure_file(netdata.sources.in netdata-edge.sources @ONLY)
  message(STATUS "Preparing changelogs")
  set(PKG_NAME netdata-repo)
  file(MAKE_DIRECTORY ${CMAKE_BINARY_DIR}/netdata-repo)
  configure_file(deb.changelog netdata-repo/changelog @ONLY)
  set(PKG_NAME netdata-repo-edge)
  file(MAKE_DIRECTORY ${CMAKE_BINARY_DIR}/netdata-repo-edge)
  configure_file(deb.changelog netdata-repo-edge/changelog @ONLY)

  install(FILES ${CMAKE_BINARY_DIR}/netdata.sources
          DESTINATION etc/apt/sources.list.d
          COMPONENT netdata-repo)
  install(FILES ${CMAKE_BINARY_DIR}/netdata-edge.sources
          DESTINATION etc/apt/sources.list.d
          COMPONENT netdata-repo-edge)

  add_custom_command(OUTPUT ${CMAKE_BINARY_DIR}/netdatabot.gpg.key
                     COMMENT "Fetch GPG key from ${DEB_GPG_KEY_SOURCE}"
                     COMMAND ${CURL} -f -L -o ${CMAKE_BINARY_DIR}/netdatabot.gpg.key ${DEB_GPG_KEY_SOURCE})

  add_custom_command(OUTPUT ${CMAKE_BINARY_DIR}/netdata.gpg
                     COMMENT "Dearmor ${CMAKE_BINARY_DIR}/netdatabot.gpg.key"
                     DEPENDS ${CMAKE_BINARY_DIR}/netdatabot.gpg.key
                     COMMAND ${GPG} --dearmor --output ${CMAKE_BINARY_DIR}/netdata.gpg ${CMAKE_BINARY_DIR}/netdatabot.gpg.key)
  add_custom_target(dearmor_gpg_key
                    ALL
                    COMMENT "Dearmor ${CMAKE_BINARY_DIR}/netdatabot.gpg.key"
                    DEPENDS ${CMAKE_BINARY_DIR}/netdata.gpg)

  install(FILES ${CMAKE_BINARY_DIR}/netdata.gpg
          DESTINATION usr/share/keyrings
          RENAME netdata-archive-keyring.gpg
          PERMISSIONS OWNER_READ GROUP_READ WORLD_READ
          COMPONENT netdata-repo)

  install(FILES ${CMAKE_BINARY_DIR}/netdata.gpg
          DESTINATION usr/share/keyrings
          RENAME netdata-archive-keyring.gpg
          PERMISSIONS OWNER_READ GROUP_READ WORLD_READ
          COMPONENT netdata-repo-edge)

  set(CPACK_DEB_COMPONENT_INSTALL YES)
  set(CPACK_DEBIAN_DEBUGINFO_PACKAGE NO)
  set(CPACK_DEBIAN_PACKAGE_SHLIBDEPS NO)
  set(CPACK_DEBIAN_ENABLE_COMPONENT_DEPENDS YES)
  set(CPACK_DEBIAN_FILE_NAME DEB-DEFAULT)
  set(CPACK_DEBIAN_PACKAGE_MAINTAINER "Netdata Builder <bot@netdata.cloud>")
  set(CPACK_DEBIAN_PACKAGE_ARCHITECTURE "all")
  set(CPACK_DEBIAN_PACKAGE_DEPENDS "debian-keyring, gnupg")
  set(CPACK_DEBIAN_PACKAGE_SECTION "net")
  set(CPACK_DEBIAN_PACKAGE_VERSION "${PACKAGE_VERSION}")
  set(CPACK_DEBIAN_PACKAGE_RELEASE "${PACKAGE_RELEASE}")
  set(CPACK_DEBIAN_NETDATA-REPO_PACKAGE_NAME "netdata-repo")
  set(CPACK_DEBIAN_NETDATA-REPO-EDGE_PACKAGE_NAME "netdata-repo-edge")
  set(CPACK_DEBIAN_NETDATA-REPO_PACKAGE_CONFLICTS "netdata-repo-edge")
  set(CPACK_DEBIAN_NETDATA-REPO-EDGE_PACKAGE_CONFLICTS "netdata-repo")
  set(CPACK_DEBIAN_NETDATA-REPO_PACKAGE_CONTROL_EXTRA "${CMAKE_BINARY_DIR}/netdata-repo/changelog")
  set(CPACK_DEBIAN_NETDATA-REPO-EDGE_PACKAGE_CONTROL_EXTRA "${CMAKE_BINARY_DIR}/netdata-repo-edge/changelog")
elseif(${DISTRO} IN_LIST RPM_DISTROS)
  require_command(RPM rpm)

  if(${RPM} STREQUAL "RPM-NOTFOUND")
    message(FATAL_ERROR "Unable to find rpm, which is required for RPM package builds.")
  endif()

  set(REPO_CFG_PATH "yum.repos.d")
  set(REPO_ID "dnf")
  set(DIST_NAME "${DISTRO}")
  set(DIST_VERSION "$releasever")

  if(${DISTRO} STREQUAL "amzn")
    set(DIST_NAME "amazonlinux")
    if(${DISTRO_VERSION} VERSION_EQUAL 2)
      # Nothing to do in this case, defaults work here.
    elseif(${DISTRO_VERSION} VERSION_EQUAL 2023)
      set(DIST_VERSION "2023")
    else()
      message(FATAL_ERROR "Unsupported version of Amazon Linux: ${DISTRO_VERSION}")
    endif()
  elseif(${DISTRO} STREQUAL "opensuse")
    set(REPO_CFG_PATH "zypp/repos.d")
    set(REPO_ID "zypp")
    set(DIST_NAME "opensuse")
  elseif(${DISTRO} STREQUAL "rhel")
    set(DIST_NAME "el")

    if(${DISTRO_VERSION} VERSION_LESS_EQUAL 8)
      set(CPACK_RPM_PACKAGE_REQUIRES "yum-plugin-priorities, epel-release")
    else()
      set(CPACK_RPM_PACKAGE_REQUIRES "epel-release")
    endif()
  endif()

  message(STATUS "Generating stable repository configuration for ${DISTRO} ${DISTRO_VERSION}")
  set(VARIANT stable)
  configure_file(netdata.repo.${REPO_ID} netdata.repo @ONLY)
  message(STATUS "Generating edge repository configuration for ${DISTRO} ${DISTRO_VERSION}")
  set(VARIANT edge)
  configure_file(netdata.repo.${REPO_ID} netdata-edge.repo @ONLY)

  install(FILES ${CMAKE_BINARY_DIR}/netdata.repo
          COMPONENT netdata-repo
          DESTINATION etc/${REPO_CFG_PATH})
  install(FILES ${CMAKE_BINARY_DIR}/netdata-edge.repo
          COMPONENT netdata-repo-edge
          DESTINATION etc/${REPO_CFG_PATH})

  set(CPACK_RPM_COMPONENT_INSTALL ON)
  set(CPACK_RPM_PACAKGE_AUTOREQPROV OFF)
  set(CPACK_RPM_DEBUGINFO_PACKAGE OFF)
  set(CPACK_RPM_PACKAGE_LICENSE "GPLv2")
  set(CPACK_RPM_PACKAGE_GROUP "System Environment/Base")
  set(CPACK_RPM_EXCLUDE_FROM_AUTO_FILELIST "")
  set(CPACK_RPM_PACKAGE_ARCHITECTURE "noarch")
  set(CPACK_RPM_PACKAGE_VERSION "${PACKAGE_VERSION}")
  set(CPACK_RPM_PACKAGE_RELEASE "${PACKAGE_RELEASE}")
  set(CPACK_RPM_PACKAGE_CHANGELOG "${CMAKE_SOURCE_DIR}/rpm.changelog")
  set(CPACK_RPM_NETDATA-REPO_FILE_NAME "netdata-repo-${PACKAGE_VERSION}-${PACKAGE_RELEASE}.noarch.rpm")
  set(CPACK_RPM_NETDATA-REPO_PACKAGE_NAME "netdata-repo")
  set(CPACK_RPM_NETDATA-REPO_PACAKGE_CONFLICTS "netdata-repo-edge")
  set(CPACK_RPM_NETDATA-REPO-EDGE_FILE_NAME "netdata-repo-edge-${PACKAGE_VERSION}-${PACKAGE_RELEASE}.noarch.rpm")
  set(CPACK_RPM_NETDATA-REPO-EDGE_PACKAGE_NAME "netdata-repo-edge")
  set(CPACK_RPM_NETDATA-REPO-EDGE_PACKAGE_CONFLICTS "netdata-repo")
else()
  message(FATAL_ERROR "Unsupported distribution ${DISTRO} ${DISTRO_VERSION}")
endif()

include(CPack)
