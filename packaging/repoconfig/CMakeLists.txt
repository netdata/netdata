# SPDX-License-Identifier: GPL-3.0-or-later

cmake_minimum_required(VERSION 3.16.0...3.30)

list(APPEND RHEL_DISTROS centos centos-stream rhel rocky almalinux cloudlinux)
list(APPEND SUSE_DISTROS opensuse-leap opensuse-tumbleweed)
list(APPEND RPM_DISTROS rhel opensuse ol amzn fedora)
list(APPEND DEB_DISTROS debian ubuntu)

set(DEB_GPG_KEY_SOURCE "https://repo.netdata.cloud/netdatabot.gpg.key")

set(PACKAGE_VERSION 5)
set(PACKAGE_RELEASE 6)

set(CPACK_THREADS 0)
set(CPACK_STRIP_FILES NO)
set(CPACK_PACKAGE_INSTALL_DIRECTORY "netdata")
set(CPACK_PACKAGE_DIRECTORY "${CMAKE_BINARY_DIR}/packages")
set(CPACK_PACKAGING_INSTALL_PREFIX "/")
set(CPACK_PACKAGE_VENDOR "Netdata Inc.")
# Stable retains its original package and configuration file names.
set(REPO_VARIANTS stable edge preview)
foreach(VARIANT IN LISTS REPO_VARIANTS)
  if(VARIANT STREQUAL "stable")
    set(REPO_SUFFIX "")
  else()
    set(REPO_SUFFIX "-${VARIANT}")
  endif()
  set(REPO_PACKAGE_${VARIANT} "netdata-repo${REPO_SUFFIX}")
  set(REPO_CONFIG_${VARIANT} "netdata${REPO_SUFFIX}")
  string(TOUPPER "${REPO_PACKAGE_${VARIANT}}" COMPONENT_NAME)
  set(CPACK_COMPONENT_${COMPONENT_NAME}_DESCRIPTION
      "Configuration for the official Netdata ${VARIANT} package repository.")
endforeach()

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
  string(FIND "${DISTRO_VERSION}" "." DECIMAL_INDEX)
  if("${DECIMAL_INDEX}" GREATER 0)
    string(SUBSTRING "${DISTRO_VERSION}" 0 "${DECIMAL_INDEX}" DISTRO_VERSION)
  endif()
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
  foreach(VARIANT IN LISTS REPO_VARIANTS)
    message(STATUS "Generating ${VARIANT} repository configuration for ${DISTRO} ${SUITE}")
    set(PKG_NAME "${REPO_PACKAGE_${VARIANT}}")
    set(CONFIG_NAME "${REPO_CONFIG_${VARIANT}}")
    configure_file(netdata.sources.in ${CONFIG_NAME}.sources @ONLY)
    file(MAKE_DIRECTORY ${CMAKE_BINARY_DIR}/${PKG_NAME})
    configure_file(deb.changelog ${PKG_NAME}/changelog @ONLY)

    install(FILES ${CMAKE_BINARY_DIR}/${CONFIG_NAME}.sources
            DESTINATION etc/apt/sources.list.d
            COMPONENT ${PKG_NAME})
    install(FILES ${CMAKE_BINARY_DIR}/netdata.gpg
            DESTINATION usr/share/keyrings
            RENAME netdata-archive-keyring.gpg
            PERMISSIONS OWNER_READ GROUP_READ WORLD_READ
            COMPONENT ${PKG_NAME})
  endforeach()

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
  foreach(VARIANT IN LISTS REPO_VARIANTS)
    set(PKG_NAME "${REPO_PACKAGE_${VARIANT}}")
    string(TOUPPER "${PKG_NAME}" COMPONENT_NAME)
    set(CONFLICTS netdata-repo netdata-repo-edge netdata-repo-preview)
    list(REMOVE_ITEM CONFLICTS "${PKG_NAME}")
    list(JOIN CONFLICTS ", " CONFLICTS)
    set(CPACK_DEBIAN_${COMPONENT_NAME}_PACKAGE_NAME "${PKG_NAME}")
    set(CPACK_DEBIAN_${COMPONENT_NAME}_PACKAGE_CONFLICTS "${CONFLICTS}")
    set(CPACK_DEBIAN_${COMPONENT_NAME}_PACKAGE_CONTROL_EXTRA "${CMAKE_BINARY_DIR}/${PKG_NAME}/changelog")
  endforeach()
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
      set(DIST_VERSION "2")
    elseif(${DISTRO_VERSION} VERSION_EQUAL 2023)
      set(DIST_VERSION "2023")
    else()
      message(FATAL_ERROR "Unsupported version of Amazon Linux: ${DISTRO_VERSION}")
    endif()
  elseif(${DISTRO} STREQUAL "opensuse")
    set(REPO_CFG_PATH "zypp/repos.d")
    set(REPO_ID "zypp")
    set(DIST_NAME "opensuse")

    if(${DISTRO_VERSION} STREQUAL tumbleweed)
        set(DIST_VERSION "tumbleweed")
    endif()
  elseif(${DISTRO} STREQUAL "rhel")
    set(DIST_NAME "el")
    set(DIST_VERSION "${DISTRO_VERSION}")
    set(CPACK_RPM_PACKAGE_REQUIRES "epel-release")

    if(${DISTRO_VERSION} VERSION_LESS 8)
      set(CPACK_RPM_PACKAGE_REQUIRES "yum-plugin-priorities, epel-release")
    endif()
  endif()

  foreach(VARIANT IN LISTS REPO_VARIANTS)
    message(STATUS "Generating ${VARIANT} repository configuration for ${DISTRO} ${DISTRO_VERSION}")
    set(CONFIG_NAME "${REPO_CONFIG_${VARIANT}}")
    configure_file(netdata.repo.${REPO_ID} ${CONFIG_NAME}.repo @ONLY)
    install(FILES ${CMAKE_BINARY_DIR}/${CONFIG_NAME}.repo
            COMPONENT ${REPO_PACKAGE_${VARIANT}}
            DESTINATION etc/${REPO_CFG_PATH})
  endforeach()

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
  foreach(VARIANT IN LISTS REPO_VARIANTS)
    set(PKG_NAME "${REPO_PACKAGE_${VARIANT}}")
    string(TOUPPER "${PKG_NAME}" COMPONENT_NAME)
    set(CONFLICTS netdata-repo netdata-repo-edge netdata-repo-preview)
    list(REMOVE_ITEM CONFLICTS "${PKG_NAME}")
    list(JOIN CONFLICTS ", " CONFLICTS)
    set(CPACK_RPM_${COMPONENT_NAME}_FILE_NAME "${PKG_NAME}-${PACKAGE_VERSION}-${PACKAGE_RELEASE}.noarch.rpm")
    set(CPACK_RPM_${COMPONENT_NAME}_PACKAGE_NAME "${PKG_NAME}")
    set(CPACK_RPM_${COMPONENT_NAME}_PACKAGE_CONFLICTS "${CONFLICTS}")
  endforeach()
else()
  message(FATAL_ERROR "Unsupported distribution ${DISTRO} ${DISTRO_VERSION}")
endif()

include(CPack)
