# SPDX-License-Identifier: GPL-3.0-or-later
# Derives the Netdata version: git-describe when the checkout allows it,
# packaging/version otherwise. Feeds project() in the root file,
# CPACK_PACKAGE_VERSION in Packaging.cmake, and the NETDATA_VERSION defines
# in config.h.
#
# Version forms (see .github/RELEASING.md):
#
#   input                    type               package version          project (MSI) version (always four
#                                                                                fields for Win32 manifests)
#   vX.Y.Z                   stable             X.Y.Z                    X.Y.(100+Z).0
#   vX.Y.Z-rcNN              release-candidate  X.Y.Z~rcNN               X.Y.NN.0
#   vX.Y.Z-rcNN-C-gH         git                X.Y.Z~rcNN.git.C.gH      X.Y.NN.C
#   vX.Y-N-nightly           nightly            X.Y.0~nightly.N          X.Y.0.N
#   vX.Y.Z-N-post-nightly    nightly            X.Y.Z.post.N             X.Y.(100+Z).N
#   vX.Y.Z-N-nightly         nightly (legacy, treated as post)
#   vX.Y[-C-gH]              git                X.Y.0~git.C[.gH]         X.Y.0.C
#   vX.Y.Z-C-gH              git                X.Y.Z.git.C.gH           X.Y.(100+Z).C
#
# vX.Y tags are development markers on master. The ~ forms sort before the
# matching release in DEB and RPM. MSI only compares the first three fields, so
# stable releases are offset by 100 to sort after release candidates (rc01-rc99).

include_guard()

# Parse VERSION into NETDATA_VERSION_* and NETDATA_{PACKAGE,PROJECT}_VERSION in the caller's scope.
function(netdata_parse_version version)
  set(num "(0|[1-9][0-9]*)")
  set(post FALSE)
  set(git_hash "")

  if(version MATCHES "^v?${num}\\.${num}\\.${num}$")
    set(type "stable")
    set(major ${CMAKE_MATCH_1})
    set(minor ${CMAKE_MATCH_2})
    set(patch ${CMAKE_MATCH_3})
    set(tweak 0)
    set(descr "N/A")
  elseif(version MATCHES "^v?${num}\\.${num}\\.${num}-rc([0-9]+)(-([0-9]+)-(g[0-9a-f]+))?$")
    set(major ${CMAKE_MATCH_1})
    set(minor ${CMAKE_MATCH_2})
    set(patch ${CMAKE_MATCH_3})
    set(rc ${CMAKE_MATCH_4})
    if(CMAKE_MATCH_5)
      set(type "git")
      set(tweak ${CMAKE_MATCH_6})
      set(git_hash ${CMAKE_MATCH_7})
      set(descr "rc${rc}.${git_hash}")
    else()
      set(type "release-candidate")
      set(tweak 0)
      set(descr "N/A")
    endif()
  elseif(version MATCHES "^v?${num}\\.${num}-([0-9]+)-nightly$")
    set(type "nightly")
    set(major ${CMAKE_MATCH_1})
    set(minor ${CMAKE_MATCH_2})
    set(patch 0)
    set(tweak ${CMAKE_MATCH_3})
    set(descr "nightly")
  elseif(version MATCHES "^v?${num}\\.${num}\\.${num}-([0-9]+)-(post-)?nightly$")
    set(type "nightly")
    set(post TRUE)
    set(major ${CMAKE_MATCH_1})
    set(minor ${CMAKE_MATCH_2})
    set(patch ${CMAKE_MATCH_3})
    set(tweak ${CMAKE_MATCH_4})
    set(descr "post-nightly")
  elseif(version MATCHES "^v?${num}\\.${num}(-([0-9]+)-(g[0-9a-f]+))?$")
    set(type "git")
    set(major ${CMAKE_MATCH_1})
    set(minor ${CMAKE_MATCH_2})
    set(patch 0)
    if(CMAKE_MATCH_3)
      set(tweak ${CMAKE_MATCH_4})
      set(git_hash ${CMAKE_MATCH_5})
    else()
      set(tweak 0)
    endif()
    set(descr "${git_hash}")
  elseif(version MATCHES "^v?${num}\\.${num}\\.${num}-([0-9]+)-(g[0-9a-f]+)$")
    set(type "git")
    set(post TRUE)
    set(major ${CMAKE_MATCH_1})
    set(minor ${CMAKE_MATCH_2})
    set(patch ${CMAKE_MATCH_3})
    set(tweak ${CMAKE_MATCH_4})
    set(git_hash ${CMAKE_MATCH_5})
    set(descr "${git_hash}")
  else()
    message(FATAL_ERROR "Unrecognized Netdata version '${version}'")
  endif()

  # Leading zeros would make these fields invalid in project() and MSI versions.
  math(EXPR tweak "${tweak}")
  if(git_hash)
    set(git_suffix ".${git_hash}")
  else()
    set(git_suffix "")
  endif()
  math(EXPR stable_patch "100 + ${patch}")

  if(type STREQUAL "stable")
    set(package_version "${major}.${minor}.${patch}")
    set(project_version "${major}.${minor}.${stable_patch}.0")
  elseif(type STREQUAL "release-candidate")
    math(EXPR rc_number "${rc}")
    set(package_version "${major}.${minor}.${patch}~rc${rc}")
    set(project_version "${major}.${minor}.${rc_number}.0")
    set(tweak ${rc_number})
  elseif(DEFINED rc)
    math(EXPR rc_number "${rc}")
    set(package_version "${major}.${minor}.${patch}~rc${rc}.git.${tweak}${git_suffix}")
    set(project_version "${major}.${minor}.${rc_number}.${tweak}")
  elseif(type STREQUAL "nightly" AND post)
    set(package_version "${major}.${minor}.${patch}.post.${tweak}")
    set(project_version "${major}.${minor}.${stable_patch}.${tweak}")
  elseif(type STREQUAL "nightly")
    set(package_version "${major}.${minor}.0~nightly.${tweak}")
    set(project_version "${major}.${minor}.0.${tweak}")
  elseif(post)
    set(package_version "${major}.${minor}.${patch}.git.${tweak}${git_suffix}")
    set(project_version "${major}.${minor}.${stable_patch}.${tweak}")
  else()
    set(package_version "${major}.${minor}.0~git.${tweak}${git_suffix}")
    set(project_version "${major}.${minor}.0.${tweak}")
  endif()

  set(NETDATA_VERSION_MAJOR "${major}" PARENT_SCOPE)
  set(NETDATA_VERSION_MINOR "${minor}" PARENT_SCOPE)
  set(NETDATA_VERSION_PATCH "${patch}" PARENT_SCOPE)
  set(NETDATA_VERSION_TWEAK "${tweak}" PARENT_SCOPE)
  set(NETDATA_VERSION_DESCR "${descr}" PARENT_SCOPE)
  set(NETDATA_VERSION_TYPE "${type}" PARENT_SCOPE)
  set(NETDATA_PACKAGE_VERSION "${package_version}" PARENT_SCOPE)
  set(NETDATA_PROJECT_VERSION "${project_version}" PARENT_SCOPE)
endfunction()

function(netdata_version)
  # Official builds carry this generated identity through archives and images.
  # It is separate from the existing display and native package versions.
  foreach(field VERSION CHANNEL REF)
    if(DEFINED NETDATA_RELEASE_${field} AND NOT "${NETDATA_RELEASE_${field}}" STREQUAL "")
      message(FATAL_ERROR "Release identity must come from packaging/release-info, not NETDATA_RELEASE_${field}")
    endif()
    set(NETDATA_RELEASE_${field} "" PARENT_SCOPE)
  endforeach()
  set(release_info "${CMAKE_SOURCE_DIR}/packaging/release-info")
  if(EXISTS "${release_info}")
    file(SIZE "${release_info}" release_size)
    if(release_size GREATER 256)
      message(FATAL_ERROR "packaging/release-info exceeds its size limit")
    endif()
    file(READ "${release_info}" release_body)
    # Match the stamper's fixed three-line format; never evaluate file contents.
    file(READ "${release_info}" release_hex HEX)
    if(release_hex MATCHES "^(..)*(00|0d)" OR NOT release_body MATCHES
       "^version=([A-Za-z0-9][A-Za-z0-9._-]*)\nchannel=(stable|nightly|devel)\nsource=([0-9a-f]+)\n$")
      message(FATAL_ERROR "Invalid packaging/release-info: expected version, channel and source lines")
    endif()
    set(release_version "${CMAKE_MATCH_1}")
    set(release_channel "${CMAKE_MATCH_2}")
    set(release_source "${CMAKE_MATCH_3}")
    string(LENGTH "${release_version}" release_version_length)
    string(LENGTH "${release_source}" release_source_length)
    if(release_version_length GREATER 128 OR NOT release_source_length EQUAL 40)
      message(FATAL_ERROR "Invalid packaging/release-info version, channel or source")
    endif()
    set(NETDATA_RELEASE_VERSION "${release_version}" PARENT_SCOPE)
    set(NETDATA_RELEASE_CHANNEL "${release_channel}" PARENT_SCOPE)
    set(NETDATA_RELEASE_REF "${release_source}" PARENT_SCOPE)
  endif()

  find_package(Git)

  file(STRINGS "${CMAKE_SOURCE_DIR}/packaging/version" file_version)
  string(STRIP "${file_version}" file_version)

  set(version "")
  if(GIT_EXECUTABLE)
    execute_process(COMMAND ${GIT_EXECUTABLE} describe
                    WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
                    RESULT_VARIABLE describe_result
                    OUTPUT_VARIABLE describe_output
                    OUTPUT_STRIP_TRAILING_WHITESPACE)
    if(NOT describe_result)
      set(version "${describe_output}")
      # Nightly builds are untagged commits that only change packaging/version; use its nightly
      # version when HEAD is that commit, as git describe would report a git build instead.
      if(file_version MATCHES "-nightly$")
        execute_process(COMMAND ${GIT_EXECUTABLE} log -1 --format=%H -- packaging/version
                        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
                        OUTPUT_VARIABLE version_commit
                        OUTPUT_STRIP_TRAILING_WHITESPACE)
        execute_process(COMMAND ${GIT_EXECUTABLE} rev-parse HEAD
                        WORKING_DIRECTORY ${CMAKE_SOURCE_DIR}
                        OUTPUT_VARIABLE head_commit
                        OUTPUT_STRIP_TRAILING_WHITESPACE)
        if(version_commit STREQUAL head_commit)
          set(version "${file_version}")
        endif()
      endif()
    endif()
  endif()

  if(version STREQUAL "")
    set(version "${file_version}")
    message(WARNING "using version from packaging/version: '${version}'")
  endif()

  set(NETDATA_VERSION_STRING "${version}" PARENT_SCOPE)
  netdata_parse_version("${version}")

  foreach(var MAJOR MINOR PATCH TWEAK DESCR TYPE)
    set(NETDATA_VERSION_${var} "${NETDATA_VERSION_${var}}" PARENT_SCOPE)
  endforeach()
  set(NETDATA_PACKAGE_VERSION "${NETDATA_PACKAGE_VERSION}" PARENT_SCOPE)
  set(NETDATA_PROJECT_VERSION "${NETDATA_PROJECT_VERSION}" PARENT_SCOPE)
endfunction()
