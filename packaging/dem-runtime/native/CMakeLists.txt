# SPDX-License-Identifier: GPL-3.0-or-later
# Included by Agent Packaging.cmake. This is not a standalone CPack project.
if(CMAKE_SOURCE_DIR STREQUAL CMAKE_CURRENT_LIST_DIR)
  message(FATAL_ERROR "Configure the Agent source root with DEM_RUNTIME_PAYLOAD_DIR instead of this component fragment")
endif()
set(DEM_RUNTIME_PAYLOAD_DIR "" CACHE PATH "Completed, paired DEM native payload for optional native packages")
if(NOT DEM_RUNTIME_PAYLOAD_DIR)
  return()
endif()
if(NOT OS_LINUX OR NOT ENABLE_PLUGIN_DEM OR NOT NETDATA_PACKAGE_KIND MATCHES "^(deb|rpm)$")
  message(FATAL_ERROR "DEM_RUNTIME_PAYLOAD_DIR requires a Linux native package build with the DEM plugin")
endif()
if(NOT IS_ABSOLUTE "${DEM_RUNTIME_PAYLOAD_DIR}" OR NOT IS_DIRECTORY "${DEM_RUNTIME_PAYLOAD_DIR}"
   OR NOT EXISTS "${DEM_RUNTIME_PAYLOAD_DIR}/native.json"
   OR NOT EXISTS "${DEM_RUNTIME_PAYLOAD_DIR}/release-info"
   OR NOT EXISTS "${CMAKE_SOURCE_DIR}/packaging/release-info")
  message(FATAL_ERROR "DEM_RUNTIME_PAYLOAD_DIR must contain a completed native payload and paired release-info")
endif()
# CI validates native.json before invoking the distro builder. Compare the same
# canonical identity here without requiring JSON support or Python in CMake.
file(SHA256 "${DEM_RUNTIME_PAYLOAD_DIR}/release-info" dem_payload_release)
file(SHA256 "${CMAKE_SOURCE_DIR}/packaging/release-info" dem_agent_release)
if(NOT dem_payload_release STREQUAL dem_agent_release)
  message(FATAL_ERROR "DEM native payload belongs to another Agent release")
endif()

set(_dem_native_dir "${PKG_FILES_PATH}/../../dem-runtime/native")
install(DIRECTORY "${DEM_RUNTIME_PAYLOAD_DIR}/" DESTINATION usr/lib/netdata/dem-runtime
        COMPONENT dem-runtime EXCLUDE_FROM_ALL USE_SOURCE_PERMISSIONS)
install(FILES "${_dem_native_dir}/README.md" DESTINATION usr/share/doc/netdata-dem-runtime
        COMPONENT dem-runtime EXCLUDE_FROM_ALL)
install(FILES "${_dem_native_dir}/netdata-dem-browser" DESTINATION etc/apparmor.d
        COMPONENT dem-runtime EXCLUDE_FROM_ALL
        PERMISSIONS OWNER_READ OWNER_WRITE GROUP_READ WORLD_READ)

set(CPACK_COMPONENT_DEM-RUNTIME_DESCRIPTION "Optional browser runtime paired with the Netdata Agent")
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_NAME netdata-dem-runtime)
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_SECTION net)
# Keep the epoch in control metadata only: APT can silently ignore local DEB
# paths containing a colon. Resolve architecture the same way as CPackDeb.
if(NETDATA_PACKAGE_KIND STREQUAL "deb")
  set(_dem_deb_arch "${CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_ARCHITECTURE}")
  if(NOT _dem_deb_arch)
    set(_dem_deb_arch "${CPACK_DEBIAN_PACKAGE_ARCHITECTURE}")
  endif()
  if(NOT _dem_deb_arch)
    find_program(_dem_dpkg dpkg)
    if(NOT _dem_dpkg)
      message(FATAL_ERROR "DEM native DEB packaging requires dpkg or an explicit Debian architecture")
    endif()
    execute_process(COMMAND "${_dem_dpkg}" --print-architecture
                    OUTPUT_VARIABLE _dem_deb_arch OUTPUT_STRIP_TRAILING_WHITESPACE
                    RESULT_VARIABLE _dem_deb_arch_status)
    if(NOT _dem_deb_arch_status EQUAL 0 OR NOT _dem_deb_arch)
      message(FATAL_ERROR "Cannot determine the DEM native DEB architecture")
    endif()
  endif()
  set(CPACK_DEBIAN_DEM-RUNTIME_FILE_NAME "netdata-dem-runtime_${CPACK_PACKAGE_VERSION}_${_dem_deb_arch}.deb")
endif()
# Do not use COMPONENT_DEPENDS: its generated DEB dependency would inherit the
# runtime's epoch instead of the matching Agent/plugin package version.
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_DEPENDS "netdata-plugin-dem (= ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_PREDEPENDS "netdata-dem-runtime-api (= 1), init-system-helpers, systemd, coreutils, sed")
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_SHLIBDEPS OFF)
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_GENERATE_SHLIBS OFF)
set(CPACK_DEBIAN_DEM-RUNTIME_DEBUGINFO_PACKAGE OFF)
set(CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_CONTROL_STRICT_PERMISSION TRUE)

set(CPACK_RPM_DEM-RUNTIME_PACKAGE_NAME netdata-dem-runtime)
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_SUMMARY "Optional browser runtime paired with the Netdata Agent")
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES "netdata-plugin-dem = ${CPACK_RPM_PACKAGE_VERSION}, systemd, coreutils, sed")
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES_PRE "netdata-dem-runtime-api = 1, systemd, coreutils, sed")
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES_PREUN "netdata-dem-runtime-api = 1, systemd, coreutils, sed")
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_AUTOREQPROV OFF)
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_AUTOREQ OFF)
set(CPACK_RPM_DEM-RUNTIME_PACKAGE_AUTOPROV OFF)
set(CPACK_RPM_DEM-RUNTIME_DEBUGINFO_PACKAGE OFF)
set(CPACK_RPM_DEM-RUNTIME_DEFAULT_USER root)
set(CPACK_RPM_DEM-RUNTIME_DEFAULT_GROUP root)
set(CPACK_RPM_DEM-RUNTIME_USER_FILELIST "%config(noreplace) /etc/apparmor.d/netdata-dem-browser")

file(READ "${_dem_native_dir}/lifecycle.sh" NATIVE_LIFECYCLE)
set(_dem_hooks "${CMAKE_BINARY_DIR}/packaging/dem-runtime/hooks")
file(MAKE_DIRECTORY "${_dem_hooks}/deb" "${_dem_hooks}/rpm")
set(NATIVE_FAMILY deb)
foreach(hook preinst postinst prerm postrm)
  set(NATIVE_HOOK "deb-${hook}")
  configure_file("${_dem_native_dir}/hook.sh.in" "${_dem_hooks}/deb/${hook}" @ONLY)
  list(APPEND CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_CONTROL_EXTRA "${_dem_hooks}/deb/${hook}")
endforeach()
file(WRITE "${_dem_hooks}/deb/conffiles" "/etc/apparmor.d/netdata-dem-browser\n")
list(APPEND CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_CONTROL_EXTRA "${_dem_hooks}/deb/conffiles")
set(NATIVE_FAMILY rpm)
foreach(hook pre preun posttrans postun)
  set(NATIVE_HOOK "rpm-${hook}")
  configure_file("${_dem_native_dir}/hook.sh.in" "${_dem_hooks}/rpm/${hook}" @ONLY)
endforeach()
set(CPACK_RPM_DEM-RUNTIME_PRE_INSTALL_SCRIPT_FILE "${_dem_hooks}/rpm/pre")
set(CPACK_RPM_DEM-RUNTIME_PRE_UNINSTALL_SCRIPT_FILE "${_dem_hooks}/rpm/preun")
set(CPACK_RPM_DEM-RUNTIME_POST_TRANS_SCRIPT_FILE "${_dem_hooks}/rpm/posttrans")
set(CPACK_RPM_DEM-RUNTIME_POST_UNINSTALL_SCRIPT_FILE "${_dem_hooks}/rpm/postun")

# CPack's epoch and additional RPM spec headers are global. Package only this
# component in a second pass so calendar-version migration does not change any
# Agent package's epoch or its established version/revision conventions.
file(WRITE "${CMAKE_BINARY_DIR}/CPackDEMRuntimeConfig.cmake"
"include(\"${CMAKE_BINARY_DIR}/CPackConfig.cmake\")
set(CPACK_COMPONENTS_ALL dem-runtime)
set(CPACK_DEBIAN_PACKAGE_EPOCH 1)
set(CPACK_RPM_PACKAGE_EPOCH 1)
set(CPACK_PACKAGE_RELOCATABLE FALSE)
set(CPACK_RPM_PACKAGE_RELOCATABLE FALSE)
set(CPACK_DEBIAN_PACKAGE_SHLIBDEPS OFF)
set(CPACK_DEBIAN_PACKAGE_GENERATE_SHLIBS OFF)
set(CPACK_RPM_PACKAGE_LICENSE \"Multiple (see bundled component notices)\")
set(CMAKE_INSTALL_DEFAULT_DIRECTORY_PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE WORLD_READ WORLD_EXECUTE)
list(APPEND CPACK_RPM_EXCLUDE_FROM_AUTO_FILELIST_ADDITION /etc/apparmor.d)
string(APPEND CPACK_RPM_SPEC_MORE_DEFINE \"\\nRequires(posttrans): netdata-plugin-dem = ${CPACK_RPM_PACKAGE_VERSION}, netdata-dem-runtime-api = 1, systemd, coreutils, sed\")
")
