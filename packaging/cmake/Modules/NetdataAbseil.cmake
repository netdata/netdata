# Macros and functions for handling of Abseil
#
# To use:
# - Include the NetdataAbseil module
# - Call netdata_detect_abseil() with a list of the Abseil components you need.
# - Call netdata_add_abseil_to_target() with the name of the target that needs to link against Abseil.
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

# Prepare a vendored copy of Abseil for use with Netdata
function(netdata_bundle_abseil)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        string(REPLACE "-fsanitize=address" "" CMAKE_C_FLAGS ${CMAKE_C_FLAGS})
        string(REPLACE "-fsanitize=address" "" CMAKE_CXX_FLAGS ${CMAKE_CXX_FLAGS})

        # ignore debhelper
        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        set(ABSL_PROPAGATE_CXX_STD On)
        set(ABSL_ENABLE_INSTALL Off)
        set(BUILD_SHARED_LIBS Off)

        message(STATUS "Preparing bundled Abseil")
        FetchContent_Declare(absl
                GIT_REPOSITORY https://github.com/abseil/abseil-cpp
                GIT_TAG 2f9e432cce407ce0ae50676696666f33a77d42ac # 20240116.1
                CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
        )
        FetchContent_MakeAvailable_NoInstall(absl)
        message(STATUS "Finished preparing bundled Abseil")
        set(BUNDLED_ABSEIL PARENT_SCOPE)
endfunction()

# Look for a system copy of Abseil, and if we can’t find it bundle a copy.
#
# _components is a list of Abseil component libraries to look for,
# without the `absl_` prefix. All of them must be found to use a system copy.
#
# If called multiple times with different lists of components, the
# end result will be as if it was called once with the union of the sets
# of components.
function(netdata_detect_abseil _components)
        set(_components_found "${ENABLE_BUNDLED_ABSEIL}")
        set(_cflags_other "${NETDATA_ABSEIL_CFLAGS_OTHER}")
        set(_include_dirs "${NETDATA_ABSEIL_INCLUDE_DIRS}")
        set(_libs "${NETDATA_ABSEIL_LIBS}")
        set(_bundled "${BUNDLED_ABSEIL}")

        if(NOT BUNDLED_ABSEIL)
                if(NOT ENABLE_BUNDLED_ABSEIL)
                        foreach(_c LISTS _components)
                                set(_component_name "absl_${_c}")
                                pkg_check_modules("${_component_name}")

                                if(${_component_name}_FOUND)
                                        list(APPEND _cflags_other "${${_component_name}_CFLAGS_OTHER}")
                                        list(APPEND _include_dirs "${${_component_name}_INCLUDE_DIRS}")
                                        list(APPEND _libs "${${_component_name}_LIBRARIES}")
                                else()
                                        set(_components_found FALSE)
                                        break()
                                endif()
                        endforeach()
                endif()

                if(NOT _components_found)
                        set(_cflags_other "${NETDATA_ABSEIL_CFLAGS_OTHER}")
                        set(_include_dirs "${NETDATA_ABSEIL_INCLUDE_DIRS}")
                        set(_libs "${NETDATA_ABSEIL_LIBS}")
                        set(_bundled TRUE)
                        message(WARNING "Could not find all required Abseil components, vendoring it instead")
                        netdata_bundle_abseil()
                endif()
        endif()

        if(BUNDLED_ABSEIL)
                foreach(_c LISTS _components)
                        set(_target "absl::${_c}")

                        if(NOT TARGET ${_target})
                                message(FATAL_ERROR "Abseil component ${_c} was not provided by the vendored copy of Abseil.")
                        endif()

                        get_target_property(${_c}_cflags_other ${_target} INTERFACE_COMPILE_DEFINITIONS)

                        if(NOT ${_c}_cflags_other STREQUAL ${_c}_cflags_other-NOTFOUND)
                                list(APPEND _cflags_other "${${_c}_cflags_other}")
                        endif()

                        get_target_property(${_c}_include_dirs ${_target} INTERFACE_INCLUDE_DIRECTORIES)

                        if(NOT ${_c}_include_dirs STREQUAL ${_c}_include_dirs-NOTFOUND)
                                list(APPEND _include_dirs "${${_c}_include_dirs}")
                        endif()

                        list(APPEND _libs ${_target})
                endforeach()
        endif()

        foreach(_var ITEMS _cflags_other _include_dirs _libs)
                list(REMOVE_ITEM ${_var} "")
                list(REMOVE_DUPLICATES ${_var})
        endforeach()

        set(NETDATA_ABSEIL_CFLAGS_OTHER "${_cflags_other}" PARENT_SCOPE)
        set(NETDATA_ABSEIL_INCLUDE_DIRS "${_include_dirs}" PARENT_SCOPE)
        set(NETDATA_ABSEIL_LIBS "${_libs}" PARENT_SCOPE)
        set(HAVE_ABSEIL TRUE PARENT_SCOPE)
        set(BUNDLED_ABSEIL "${_bundled}" PARENT_SCOPE)
endfunction()

# Add Abseil to a specified target
#
# The _scope argument is optional, if not specified Abseil will be added as a PRIVATE dependency.
function(netdata_add_abseil_to_target _target _scope)
        if(NOT _scope)
                set(_scope PRIVATE)
        endif()

        target_compile_definitions(${_target} ${_scope} ${NETDATA_ABSEIL_CFLAGS_OTHER})
        target_include_directories(${_target} ${_scope} ${NETDATA_ABSEIL_INCLUDE_DIRS})
        target_link_libraries(${_target} ${_scope} ${NETDATA_ABSEIL_LIBS})
endfunction()
