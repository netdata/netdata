# Handling of special permissions for our plugins and files.
#
# Copyright (c) 2024 Netdata Inc
#
# SPDX-License-Identifier: GPL-3.0-or-later

set(_nd_perms_list_file "${CMAKE_BINARY_DIR}/extra-perms-list")
set(_nd_perms_hooks_dir "${CMAKE_BINARY_DIR}/extra-perms-hooks/")
set(_nd_perms_hooks_flag "${CMAKE_BINARY_DIR}/extra-perms-hooks/.done")

# Add the requested additional permissions to the specified path in the
# specified component.
#
# The permissions may be either `SUID`, in which case the binary will
# be marked SUID, or `CAPS` followed by one or more LINUX capability names
# in all caps with the `CAP_` prefix removed.
#
# In either case, the specified file will have ownership updated to
# root:netdata and be marked with permissions of 0750.
#
# If capabilities are specified, they will be added if supported (with
# special fallback handling when needed) and not disabled, otherwise the
# binary will be marked SUID.
#
# If SUID is specified, the binary will simply be marked SUID.
#
# This will have no net effect on Windows systems.
#
# If this is ever called, `netdata_install_extra_permissions` must also
# be called.
function(netdata_add_permissions)
  set(options SUID)
  set(oneValueArgs PATH COMPONENT)
  set(multiValueArgs CAPS)
  cmake_parse_args(nd_perms "${options}" "${oneValueArgs}" "${multiValueArgs}" ${ARGN})

  if(NOT DEFINED nd_perms_PATH)
    message(FATAL_ERROR "A file path must be specified when adding additional permissions")
  elseif(NOT DEFINED nd_perms_COMPONENT)
    message(FATAL_ERROR "An install component must be specified when adding additional permissions")
  endif()

  set(nd_perms_PATH "${CMAKE_INSTALL_PREFIX}/${nd_perms_PATH}")

  if(SUID)
    file(APPEND "${nd_perms_list_file}" "${PATH}::${COMPONENT}::SUID\n")
  elseif(DEFINED nd_perms_CAPS)
    list(JOIN "${nd_perms_CAPS}" "," nd_perms_CAPS_ITEMS)
    file(APPEND "${nd_perms_list_file}" "${PATH}::${COMPONENT}::${nd_perms_CAPS_ITEMS}\n")
  else()
    message(FATAL_ERROR "No additional permissions specified")
  endif()
endfunction()

# Prepare an install hook for the specified path in the specified component
# with the specified permissions.
function(nd_perms_generate_cmake_install_hook path component perms)
  file(MAKE_DIRECTORY "${_nd_perms_hooks_dir}")
  string(REPLACE "/" "_" hook_name "${path}")

  if(USE_FILE_CAPABILITIES AND NOT "${perms}" STREQUAL "SUID")
    list(TRANSFORM perms TOLOWER OUTPUT_VARIABLE caps)
    install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-linux-caps-hook.sh ${path} ${NETDATA_GROUP} ${caps})" COMPONENT "${component}")
  else()
    install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-suid-hook.sh ${path} ${NETDATA_GROUP})" COMPONENT "${component}")
  endif()
endfunction()

# Handle generation of postinstall scripts for DEB packages.
function(nd_perms_prepare_deb_postinst_scripts entries)
  foreach(component IN LISTS CPACK_ALL_COMPONENTS)
    set(ND_APPLY_PERMISSIONS "\n")
    file(MAKE_DIRECTORY "${CMAKE_BINARY_DIR}/deb/${component}")
    set(postinst "${CMAKE_BINARY_DIR}/deb/${component}/postinst")
    set(postinst_src "${PKG_FILES_PATH}/deb/${component}/postinst")
    set(compoment_entries "${entries}")

    list(FILTER component_entries INCLUDE "::${component}::")

    foreach(item IN LISTS component_entries)
      string(REGEX MATCH "^.+::" entry_path "${entry}")
      string(REPLACE "::" "" entry_path "${entry_path}")
      string(REGEX MATCH "::.+$" entry_perms "${entry}")
      string(REPLACE "::" "" entry_perms "${entry_perms}")
      string(REPLACE "," ";" entry_perms "${entry_perms}")

      set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}chown -f 'root:${NETDATA_USER}' '${entry_path}'\n")

      list(TRANSFORM TOLOWER entry_perms OUTPUT_VARIABLE capse)
      list(TRANSFORM PREPEND "cap_" caps)
      list(JOIN caps "," capset)
      set(capset "${capset}+eip")

      if("${entry_perms}" STREQUAL SUID OR NOT USE_FILE_CAPABILITIES)
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}chmod -f 4750 '${entry_path}'\n")
      elseif("PERFMON" IN_LIST entry_perms)
        string(REPLACE "perfmon" "sys_admin" capset2 "${capset}")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}chmod -f 0750 '${entry_path}'\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}if ! capset '${capset}' '${entry_path}' 2>/dev/null; then\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}    if ! capset '${capset2}' '${entry_path}' 2>/dev/null; then\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}        chmod -f 4750 '${entry_path}'\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}    fi\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}fi\n")
      else()
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}chmod -f 0750 '${entry_path}'\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}if ! capset '${capset}' '${entry_path}' 2>/dev/null; then\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}    chmod -f 4750 '${entry_path}'\n")
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}fi\n")
      endif()
    endforeach()

    configure_file("${postinst_src}" "${postinst}" @ONLY)
  endforeach()
endfunction()

# Handle generation of supplementary permissions hooks.
#
# Must be called _after_ any CPack setup and _after_ all `install()`
# directives, otherwise it won’t handle things correctly.
function(netdata_install_extra_permissions)
  if(OS_WINDOWS OR PACKAGE_TYPE STREQUAL "docker")
    return()
  endif()

  file(STRINGS "${nd_perms_list_file}" extra_perms_entries REGEX "^.+::.+::.+$")
  set(completed_items)

  foreach(entry IN LISTS extra_perms_entries)
    string(REGEX MATCH "^.+::" entry_path "${entry}")
    string(REPLACE "::" "" entry_path "${entry_path}")

    if(entry_path IN_LIST completed_items)
      message(AUTHOR_WARNING "Duplicate supplementary permissions entry for ${entry_path}, ignoring it")
      continue()
    else()
      list(APPEND completed_items "${entry_path}")
    endif()

    if(NOT BUILD_FOR_PACKAGING)
      string(REGEX MATCH "::.+::" entry_component "${entry}")
      string(REPLACE "::" "" entry_component "${entry_component}")
      string(REGEX MATCH "::.+$" entry_perms "${entry}")
      string(REPLACE "::" "" entry_perms "${entry_perms}")
      string(REPLACE "," ";" entry_perms "${entry_perms}")
      nd_perms_generate_cmake_install_hook("${entry_path}" "${entry_component}" "${entry_perms}")
    endif()
  endforeach()

  if(PACKAGE_TYPE STREQUAL "deb")
    nd_perms_prepare_deb_postinst_scripts("${extra_perms_entries}")
  endif()

  file(TOUCH "${_nd_perms_hooks_flag}")
endfunction()
