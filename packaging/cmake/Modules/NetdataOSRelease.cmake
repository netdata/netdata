# Distro detection from os-release.
#
# Shared by the install rules in the top-level CMakeLists.txt (format-specific
# staging) and by Modules/Packaging.cmake (CPack per-distro configuration), so
# it must be included before either consumer.

include_guard(GLOBAL)

set(OS_DISTRO_ID "unknown")
set(OS_VERSION_ID "unknown")
set(OS_DISTRO_ID_LIKE "")

if(OS_LINUX)
  find_file(OS_RELEASE_PATH os-release PATHS /etc /usr/lib
            NO_DEFAULT_PATH
            NO_PACKAGE_ROOT_PATH
            NO_CMAKE_PATH
            NO_CMAKE_ENVIRONMENT_PATH
            NO_SYSTEM_ENVIRONMENT_PATH
            NO_CMAKE_SYSTEM_PATH
            NO_CMAKE_INSTALL_PREFIX)

  if(NOT OS_RELEASE_PATH STREQUAL OS_RELEASE_PATH-NOTFOUND)
    file(STRINGS "${OS_RELEASE_PATH}" OS_RELEASE_LINES)

    foreach(_line IN LISTS OS_RELEASE_LINES)
      if(_line MATCHES "^ID=.*$")
        string(SUBSTRING "${_line}" 3 -1 OS_DISTRO_ID)
        string(REPLACE "\"" "" OS_DISTRO_ID "${OS_DISTRO_ID}")
      elseif(_line MATCHES "^VERSION_ID=.*$")
        string(SUBSTRING "${_line}" 11 -1 OS_VERSION_ID)
        string(REPLACE "\"" "" OS_VERSION_ID "${OS_VERSION_ID}")
      elseif(_line MATCHES "^ID_LIKE=.*$")
        string(SUBSTRING "${_line}" 8 -1 OS_DISTRO_ID_LIKE)
        string(REPLACE "\"" "" OS_DISTRO_ID_LIKE "${OS_DISTRO_ID_LIKE}")
      endif()
    endforeach()
  endif()
endif()

# RPM distro-family predicates mirroring the macro families netdata.spec.in
# keys its conditionals on (%{suse_version}, %{centos_ver}/%{rhel},
# %{fedora}, %{amazon_linux}). Order matters: Amazon Linux carries
# ID_LIKE="centos rhel fedora" and must not be classified as EL.
set(NETDATA_DISTRO_SUSE FALSE)
set(NETDATA_DISTRO_EL FALSE)
set(NETDATA_DISTRO_FEDORA FALSE)
set(NETDATA_DISTRO_AMZN FALSE)
set(NETDATA_DISTRO_VERSION_MAJOR 0)

if(OS_VERSION_ID MATCHES "^([0-9]+)")
  set(NETDATA_DISTRO_VERSION_MAJOR "${CMAKE_MATCH_1}")
endif()

if(OS_DISTRO_ID STREQUAL "amzn")
  set(NETDATA_DISTRO_AMZN TRUE)
elseif(OS_DISTRO_ID STREQUAL "fedora")
  set(NETDATA_DISTRO_FEDORA TRUE)
elseif(OS_DISTRO_ID MATCHES "suse" OR OS_DISTRO_ID_LIKE MATCHES "suse")
  set(NETDATA_DISTRO_SUSE TRUE)
elseif(OS_DISTRO_ID MATCHES "^(rhel|centos|almalinux|rocky|ol|oraclelinux)$"
       OR OS_DISTRO_ID_LIKE MATCHES "rhel|centos")
  set(NETDATA_DISTRO_EL TRUE)
endif()

# Keep this classification in sync with the distro case in
# packaging/build-package.sh (RPM section).
