# Handling of special permissions for our plugins and files.
#
# Copyright (c) 2024 Netdata Inc
#
# SPDX-License-Identifier: GPL-3.0-or-later

set(_nd_perms_list_file "${CMAKE_BINARY_DIR}/extra-perms-list")
set(_nd_perms_hooks_dir "${CMAKE_BINARY_DIR}/extra-perms-hooks/")
file(REMOVE "${_nd_perms_list_file}")

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

# Prepare an install hook for the specified path in the specified component
# with the specified permissions.
function(nd_perms_generate_cmake_install_hook path component perms)
  file(MAKE_DIRECTORY "${_nd_perms_hooks_dir}")
  string(REPLACE "/" "_" hook_name "${path}")

  message(STATUS "Adding post-install hook for supplementary permissions for ${path}")

  if(USE_FILE_CAPABILITIES AND NOT "${perms}" STREQUAL "suid")
    install(CODE "execute_process(COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/install-linux-caps-hook.sh ${path} ${NETDATA_GROUP} ${perms})" COMPONENT "${component}")
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
      _nd_extract_path(entry_path "${item}")
      _nd_extract_permissions(entry_perms "${item}")

      set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}chown -f 'root:${NETDATA_USER}' '${entry_path}'\n")

      list(TRANSFORM entry_perms PREPEND "cap_")
      list(JOIN entry_perms "," capset)
      set(capset "${capset}+eip")

      if("${entry_perms}" STREQUAL "suid" OR NOT USE_FILE_CAPABILITIES)
        set(ND_APPLY_PERMISSIONS "${ND_APPLY_PERMISSIONS}chmod -f 4750 '${entry_path}'\n")
      elseif("perfmon" IN_LIST entry_perms)
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

  file(STRINGS "${_nd_perms_list_file}" extra_perms_entries REGEX "^.+::.+::.+$")
  set(completed_items)

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
      nd_perms_generate_cmake_install_hook("${entry_path}" "${entry_component}" "${entry_perms}")
    endif()
  endforeach()

  if(PACKAGE_TYPE STREQUAL "deb")
    nd_perms_prepare_deb_postinst_scripts("${extra_perms_entries}")
  endif()
endfunction()
