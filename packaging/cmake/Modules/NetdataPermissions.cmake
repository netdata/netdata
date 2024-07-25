# Handling of special permissions for our plugins and files.
#
# Copyright (c) 2024 Netdata Inc
#
# SPDX-License-Identifier: GPL-3.0-or-later

set(_nd_perms_list_file "${CMAKE_BINARY_DIR}/extra-perms-list")
set(_nd_perms_hooks_dir "${CMAKE_BINARY_DIR}/extra-perms-hooks/")
file(REMOVE "${_nd_perms_list_file}")

if(CREATE_USER)
  if(OS_LINUX)
    message(CHECK_START "Looking for getent")
    find_program(GETENT_CMD getent NO_CMAKE_INSTALL_PREFIX)
    if(GETENT_CMD STREQUAL "GETENT_CMD-NOTFOUND")
      message(CHECK_FAIL "${GETENT_CMD}")
      message(FATAL_ERROR "Could not find command to look up user accounts")
    else()
      message(CHECK_PASS "${GETENT_CMD}")
    endif()

    message(CHECK_START "Looking for useradd")
    find_program(USERADD_CMD useradd NO_CMAKE_INSTALL_PREFIX)
    if(USERADD_CMD STREQUAL "USERADD_CMD-NOTFOUND")
      message(CHECK_FAIL "failed")

      message(CHECK_START "Looking for adduser")
      find_program(ADDUSER_CMD adduser NO_CMAKE_INSTALL_PREFIX)
      if(ADDUSER_CMD STREQUAL "ADDUSER_CMD-NOTFOUND")
        message(CHECK_FAIL "failed")
        message(FATAL_ERROR "Could not find command to create accounts")
      else()
        message(CHECK_PASS "${ADDUSER_CMD}")

        message(CHECK_START "Looking for addgroup")
        find_program(ADDGROUP_CMD addgroup NO_CMAKE_INSTALL_PREFIX)
        if(ADDGROUP_CMD STREQUAL "ADDGROUP_CMD-NOTFOUND")
          message(CHECK_FAIL "failed")
          message(FATAL_ERROR "Could not find command to create groups")
        else()
          message(CHECK_PASS "${ADDGROUP_CMD}")
        endif()
      endif()
    else()
      message(CHECK_PASS "${USERADD_CMD}")

      message(CHECK_START "Looking for groupadd")
      find_program(GROUPADD_CMD groupadd NO_CMAKE_INSTALL_PREFIX)
      if(GROUPADD_CMD STREQUAL "GROUPADD_CMD-NOTFOUND")
        message(CHECK_FAIL "failed")
        message(FATAL_ERROR "Could not find command to create groups")
      else()
        message(CHECK_PASS "${GROUPADD_CMD}")
      endif()
    endif()
  elseif(OS_FREEBSD)
    message(CHECK_START "Looking for getent")
    find_program(GETENT_CMD getent NO_CMAKE_INSTALL_PREFIX)
    if(GETENT_CMD STREQUAL "GETENT_CMD-NOTFOUND")
      message(CHECK_FAIL "${GETENT_CMD}")
      message(FATAL_ERROR "Could not find command to look up user accounts")
    else()
      message(CHECK_PASS "${GETENT_CMD}")
    endif()

    message("Looking for pw")
    find_program(PW_CMD pw NO_CMAKE_INSTALL_PREFIX)
    if(PW_CMD STREQUAL "PW_CMD-NOTFOUND")
      message(CHECK_FAIL "failed")
      message(FATAL_ERROR "Could not find command to create accounts")
    else()
      message(CHECK_PASS "${PW_CMD}")
    endif()
  elseif(OS_MACOS)
    message("Looking for dscl")
    find_program(DSCL_CMD dscl NO_CMAKE_INSTALL_PREFIX)
    if(DSCL_CMD STREQUAL "DSCL_CMD-NOTFOUND")
      message(CHECK_FAIL "failed")
      message(FATAL_ERROR "Could not find dscl (needed for user account creation)")
    else()
      message(CHECK_PASS "${DSCL_CMD}")
    endif()

    message("Looking for sysadminctl")
    find_program(SYSADMINCTL_CMD sysadminctl NO_CMAKE_INSTALL_PREFIX)
    if(SYSADMINCTL_CMD STREQUAL "SYSADMINCTL_CMD-NOTFOUND")
      message(CHECK_FAIL "failed")
      message(FATAL_ERROR "Could not find sysadminctl (needed for user account creation)")
    else()
      message(CHECK_PASS "${SYSADMINCTL_CMD}")
    endif()

    message("Looking for dseditgroup")
    find_program(DSEDITGROUP_CMD dseditgroup NO_CMAKE_INSTALL_PREFIX)
    if(DSEDITGROUP_CMD STREQUAL "DSEDITGROUP_CMD-NOTFOUND")
      message(CHECK_FAIL "failed")
      message(FATAL_ERROR "Could not find dseditgroup (needed for group account creation)")
    else()
      message(CHECK_PASS "${DSEDITGROUP_CMD}")
    endif()
  else()
    message(CHECK_FAIL "failed")
    message(WARNING "Unable to determine how to create accounts on this system, disabling account creation")
    set(CREATE_USER False)
  endif()
endif()

# Add the requested additional permissions to the specified path in the
# specified component.
#
# The permissions may be either `suid`, or one or more Linux capability
# names in lowercase with the `cap_` prefix removed.
#
# In either case, the specified file will have ownership updated to
# root:netdata and be marked with permissions of 0750.
#
# If capabilities are specified, they will be added if supported (with
# special fallback handling when needed) and not disabled, otherwise the
# binary will be marked SUID.
#
# If suid is specified, the binary will simply be marked SUID.
#
# This will have no net effect on Windows systems.
#
# If this is ever called, `netdata_install_extra_permissions` must also
# be called.
function(netdata_add_permissions)
  set(oneValueArgs PATH COMPONENT)
  set(multiValueArgs PERMISSIONS)
  cmake_parse_arguments(nd_perms "" "${oneValueArgs}" "${multiValueArgs}" ${ARGN})

  if(NOT DEFINED nd_perms_PATH)
    message(FATAL_ERROR "A file path must be specified when adding additional permissions")
  elseif(NOT DEFINED nd_perms_COMPONENT)
    message(FATAL_ERROR "An install component must be specified when adding additional permissions")
  elseif(NOT DEFINED nd_perms_PERMISSIONS OR NOT nd_perms_PERMISSIONS)
    message(FATAL_ERROR "No additional permissions specified")
  endif()

  set(nd_perms_PATH "${CMAKE_INSTALL_PREFIX}/${nd_perms_PATH}")

  list(JOIN nd_perms_PERMISSIONS "," nd_perms_PERMISSIONS_ITEMS)
  file(APPEND "${_nd_perms_list_file}" "${nd_perms_PATH}::${nd_perms_COMPONENT}::${nd_perms_PERMISSIONS_ITEMS}\n")
endfunction()

# Extract the path from a supplementary permissions entry.
function(_nd_extract_path var entry)
  string(REGEX MATCH "^.+::" entry_path "${entry}")
  string(REGEX REPLACE "::$" "" entry_path "${entry_path}")
  string(REGEX MATCH "^.+::" entry_path "${entry_path}")
  string(REPLACE "::" "" entry_path "${entry_path}")
  set(${var} "${entry_path}" PARENT_SCOPE)
endfunction()

# Extract the component from a supplementary permissions entry.
function(_nd_extract_component var entry)
  string(REGEX MATCH "::.+::" entry_component "${entry}")
  string(REPLACE "::" "" entry_component "${entry_component}")
  set(${var} "${entry_component}" PARENT_SCOPE)
endfunction()

# Extract the permissions list from a supplementary permissions entry.
function(_nd_extract_permissions var entry)
  string(REGEX MATCH "::.+$" entry_perms "${entry}")
  string(REGEX REPLACE "^::" "" entry_perms "${entry_perms}")
  string(REGEX MATCH "::.+$" entry_perms "${entry_perms}")
  string(REPLACE "::" "" entry_perms "${entry_perms}")
  string(REPLACE "," ";" entry_perms "${entry_perms}")
  set(${var} "${entry_perms}" PARENT_SCOPE)
endfunction()

# Add shell script to the specified variable to handle marking the
# specified path SUID.
function(_nd_perms_mark_path_suid var path)
  set(tmp_var "${${var}}chown -f 'root:${NETDATA_USER}' '${path}'\n")
  set(tmp_var "${tmp_var}chmod -f 4750 '${path}'\n")
  set(${var} "${tmp_var}" PARENT_SCOPE)
endfunction()

# Add shell script to the specified variable to handle marking the
# specified path with the specified filecaps.
function(_nd_perms_mark_path_filecaps var path capset)
  set(tmp_var "${${var}}chown -f 'root:${NETDATA_USER}' '${path}'\n")
  set(tmp_var "${tmpvar}chmod -f 0750 '${path}'\n")
  set(tmp_var "${tmp_var}if ! capset '${capset}' '${path}' 2>/dev/null; then\n")

  if(capset MATCHES "perfmon")
    string(REPLACE "perfmon" "sys_admin" capset2 "${capset}")
    set(tmp_var "${tmp_var}    if ! capset '${capset2}' '${entry_path}' 2>/dev/null; then\n")
    set(tmp_var "${tmp_var}        chmod -f 4750 '${entry_path}'\n")
    set(tmp_var "${tmp_var}    fi\n")
    set(tmp_var "${tmp_var}fi\n")
  else()
    set(tmp_var "${tmp_var}    chmod -f 4750 '${entry_path}'\n")
    set(tmp_var "${tmp_var}fi\n")
  endif()

  set(${var} "${tmp_var}" PARENT_SCOPE)
endfunction()

# Add shell script for the specified permissions entry to the specified
# variable.
function(_nd_perms_generate_entry_script var entry)
  _nd_extract_path(entry_path "${entry}")
  _nd_extract_permissions(entry_perms "${entry}")

  if("${entry_perms}" STREQUAL "suid" OR NOT USE_FILE_CAPABILITIES)
    _nd_perms_mark_path_suid(result "${entry_path}")
  else()
    list(TRANSFORM entry_perms PREPEND "cap_")
    list(JOIN entry_perms "," capset)
    set(capset "${capset}+eip")
    _nd_perms_mark_path_filecaps(result "${entry_path}" "${capset}")
  endif()

  set(${var} "${${var}}${result}" PARENT_SCOPE)
endfunction()

# Prepare an install hook for the specified path in the specified component
# with the specified permissions.
function(nd_perms_generate_cmake_install_hook path component perms)
  file(MAKE_DIRECTORY "${_nd_perms_hooks_dir}")
  string(REPLACE "/" "_" hook_name "${path}")

  message(STATUS "Adding post-install hook for supplementary permissions for ${path}")

  if(USE_FILE_CAPABILITIES AND NOT "${perms}" STREQUAL "suid")
    list(JOIN perms " " caps)

    install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-linux-caps-hook.sh ${path} ${NETDATA_GROUP} ${caps})" COMPONENT "${component}")
  else()
    install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-suid-hook.sh ${path} ${NETDATA_GROUP})" COMPONENT "${component}")
  endif()
endfunction()

# Handle generation of postinstall scripts for DEB packages.
function(nd_perms_prepare_deb_postinst_scripts entries)
  foreach(component IN LISTS CPACK_COMPONENTS_ALL)
    set(ND_APPLY_PERMISSIONS "\n")
    file(MAKE_DIRECTORY "${_nd_perms_hooks_dir}/deb/${component}")
    set(postinst "${_nd_perms_hooks_dir}/deb/${component}/postinst")
    set(postinst_src "${PKG_FILES_PATH}/deb/${component}/postinst")
    set(component_entries "${entries}")

    message(STATUS "Generating ${postinst}")

    list(FILTER component_entries INCLUDE REGEX "^.+::${component}::.+$")

    foreach(item IN LISTS component_entries)
      _nd_perms_generate_entry_script(ND_APPLY_PERMISSIONS "${item}")
    endforeach()

    configure_file("${postinst_src}" "${postinst}" @ONLY)
  endforeach()
endfunction()

# Generate a post-install hook script for our static builds.
function(nd_perms_prepare_static_postinstall_hook entries)
  file(MAKE_DIRECTORY "${_nd_perms_hooks_dir}/static")
  set(hook_path "${_nd_perms_hooks_dir}/static/apply-filecaps.sh")
  # This next variable is needed to work around CMake’s inability to
  # escape `$` in arguments.
  #
  # Syntax highlighting for this line does not work correctly in at
  # least some editors.
  set(euid_check [=[[ "${EUID}" -ne 0 ] || exit 0]=])

  message(STATUS "Generating ${hook_path}")

  set(hook_script "#!/bin/bash\n")
  set(hook_script "${hook_script}set -e\n")
  set(hook_script "${hook_script}${euid_check}\n")

  foreach(entry IN LISTS entries)
    _nd_perms_generate_entry_script(hook_script "${entry}")
  endforeach()

  file(WRITE "${hook_path}" "${hook_script}")
  install(PROGRAMS "${hook_path}"
          DESTINATION system
          COMPONENT netdata)
endfunction()

# Handle generation of supplementary permissions hooks.
#
# Must be called _after_ any CPack setup and _after_ all `install()`
# directives, otherwise it won’t handle things correctly.
function(netdata_install_extra_permissions)
  if(OS_WINDOWS OR PACKAGE_TYPE STREQUAL "docker")
    return()
  endif()

  file(STRINGS "${_nd_perms_list_file}" extra_perms_entries REGEX "^.+::.+::.+$")
  set(completed_items)
  set(user_components)

  if(CREATE_USER AND NOT BUILD_FOR_PACKAGING)
    install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-user-hook.sh ${NETDATA_USER} ${NETDATA_GROUP} ${CMAKE_INSTALL_PREFIX}/var/lib/netdata)" COMPONENT netdata)
    list(APPEND user_components "netdata")
  endif()

  foreach(entry IN LISTS extra_perms_entries)
    _nd_extract_path(entry_path "${entry}")

    if(entry_path IN_LIST completed_items)
      message(AUTHOR_WARNING "Duplicate supplementary permissions entry for ${entry_path}, ignoring it")
      continue()
    else()
      list(APPEND completed_items "${entry_path}")
    endif()

    if(NOT BUILD_FOR_PACKAGING)
      _nd_extract_component(entry_component "${entry}")
      _nd_extract_permissions(entry_perms "${entry}")

      if(CREATE_USER)
        if(NOT entry_component IN_LIST user_components)
          install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-user-hook.sh ${NETDATA_USER} ${NETDATA_GROUP} ${CMAKE_INSTALL_PREFIX}/var/lib/netdata)" COMPONENT "${component}")
          list(APPEND user_components "${entry_component}")
        endif()
      endif()

      nd_perms_generate_cmake_install_hook("${entry_path}" "${entry_component}" "${entry_perms}")
    endif()
  endforeach()

  if(PACKAGE_TYPE STREQUAL "deb")
    nd_perms_prepare_deb_postinst_scripts("${extra_perms_entries}")
  elseif(PACKAGE_TYPE STREQUAL "static")
    nd_perms_prepare_static_postinstall_hook("${extra_perms_entries}")
  endif()
endfunction()
