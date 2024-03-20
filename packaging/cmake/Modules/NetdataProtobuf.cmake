# Macros and functions for handling of Protobuf
#
# Copyright (c) 2024 Netdata Inc.
# SPDX-License-Identifier: GPL-3.0-or-later

macro(netdata_protobuf_21_tags)
        set(PROTOBUF_TAG f0dc78d7e6e331b8c6bb2d5283e06aa26883ca7c) # v21.12
        set(NEED_ABSL False)
endmacro()

macro(netdata_protobuf_25_tags)
        set(PROTOBUF_TAG 4a2aef570deb2bfb8927426558701e8bfc26f2a4) # v25.3
        set(NEED_ABSL True)
        set(ABSL_TAG 2f9e432cce407ce0ae50676696666f33a77d42ac) # 20240116.1
endmacro()

# Determine what version of protobuf and abseil to bundle.
#
# This is unfortunately very complicated because we support systems
# older than what Google officially supports for C++.
macro(netdata_set_bundled_protobuf_tags)
        netdata_protobuf_21_tags()

        if(NOT USE_CXX_11)
                if(CMAKE_CXX_COMPILER_ID STREQUAL GNU)
                        if(CMAKE_CXX_COMPILER_VERSION VERSION_GREATER_EQUAL 7.3.1)
                                netdata_protobuf_25_tags()
                        endif()
                elseif(CMAKE_CXX_COMPILER_ID STREQUAL Clang)
                        if(CMAKE_CXX_COMPILER_VERSION VERSION_GREATER_EQUAL 7.0.0)
                                netdata_protobuf_25_tags()
                        endif()
                elseif(CMAKE_CXX_COMPILER_ID STREQUAL AppleClang)
                        if(CMAKE_CXX_COMPILER_VERSION VERSION_GREATER_EQUAL 12)
                                netdata_protobuf_25_tags()
                        endif()
                endif()
        endif()
endmacro()

# Prepare a vendored copy of Protobuf for use with Netdata.
function(netdata_bundle_protobuf)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        netdata_set_bundled_protobuf_tags()

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        string(REPLACE "-fsanitize=address" "" CMAKE_C_FLAGS ${CMAKE_C_FLAGS})
        string(REPLACE "-fsanitize=address" "" CMAKE_CXX_FLAGS ${CMAKE_CXX_FLAGS})

        # ignore debhelper
        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        if(NEED_ABSL)
                set(ABSL_PROPAGATE_CXX_STD On)
                set(ABSL_ENABLE_INSTALL Off)

                message(STATUS "Preparing bundled Abseil (required by bundled Protobuf)")
                FetchContent_Declare(absl
                        GIT_REPOSITORY https://github.com/abseil/abseil-cpp
                        GIT_TAG ${ABSL_TAG}
                )
                FetchContent_MakeAvailable_NoInstall(absl)
                message(STATUS "Finished preparing bundled Abseil")
        endif()

        set(protobuf_INSTALL Off)
        set(protobuf_BUILD_LIBPROTOC Off)
        set(protobuf_BUILD_TESTS Off)
        set(protobuf_BUILD_SHARED_LIBS Off)

        message(STATUS "Preparing bundled Protobuf")
        FetchContent_Declare(protobuf
                GIT_REPOSITORY https://github.com/protocolbuffers/protobuf.git
                GIT_TAG ${PROTOBUF_TAG}
        )
        FetchContent_MakeAvailable_NoInstall(protobuf)
        message(STATUS "Finished preparing bundled Protobuf.")

        set(BUNDLED_PROTOBUF True PARENT_SCOPE)
endfunction()

# Handle detection of Protobuf
macro(netdata_detect_protobuf)
        if(NOT ENABLE_BUNDLED_PROTOBUF)
                if (NOT BUILD_SHARED_LIBS)
                        set(Protobuf_USE_STATIC_LIBS On)
                endif()

                # The FindProtobuf CMake module shipped by upstream CMake is
                # broken for Protobuf version 22.0 and newer because it does
                # not correctly pull in the new Abseil dependencies. Protobuf
                # itself sometimes ships a CMake Package Configuration module
                # that _does_ work correctly, so use that in preference to the
                # Find module shipped with CMake.
                #
                # The code below works by first attempting to use find_package
                # in config mode, and then checking for the existence of the
                # target we actually use that gets defined by the protobuf
                # CMake Package Configuration Module to determine if that
                # worked. A bit of extra logic is required in the case of the
                # config mode working, because some systems ship compatibility
                # logic for the old FindProtobuf module while others do not.
                #
                # Upstream bug reference: https://gitlab.kitware.com/cmake/cmake/-/issues/24321
                find_package(Protobuf CONFIG)

                if(NOT TARGET protobuf::libprotobuf)
                        message(STATUS "Could not find Protobuf using Config mode, falling back to Module mode")
                        find_package(Protobuf REQUIRED)
                endif()
        endif()

        if(TARGET protobuf::libprotobuf)
                if(NOT Protobuf_PROTOC_EXECUTABLE AND TARGET protobuf::protoc)
                        get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                IMPORTED_LOCATION_RELEASE)
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_RELWITHDEBINFO)
                        endif()
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_MINSIZEREL)
                        endif()
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_DEBUG)
                        endif()
                        if(NOT EXISTS "${Protobuf_PROTOC_EXECUTABLE}")
                                get_target_property(Protobuf_PROTOC_EXECUTABLE protobuf::protoc
                                        IMPORTED_LOCATION_NOCONFIG)
                        endif()
                        if(NOT Protobuf_PROTOC_EXECUTABLE)
                                set(Protobuf_PROTOC_EXECUTABLE protobuf::protoc)
                        endif()
                endif()

                # It is technically possible that this may still not
                # be set by this point, so we need to check it and
                # fail noisily if it isn't because the build won't
                # work without it.
                if(NOT Protobuf_PROTOC_EXECUTABLE)
                        message(FATAL_ERROR "Could not determine the location of the protobuf compiler for the detected version of protobuf.")
                endif()

                set(NETDATA_PROTOBUF_PROTOC_EXECUTABLE ${Protobuf_PROTOC_EXECUTABLE})
                set(NETDATA_PROTOBUF_LIBS protobuf::libprotobuf)
                get_target_property(NETDATA_PROTOBUF_CFLAGS_OTHER
                                    protobuf::libprotobuf
                                    INTERFACE_COMPILE_DEFINITIONS)
                get_target_property(NETDATA_PROTOBUF_INCLUDE_DIRS
                                    protobuf::libprotobuf
                                    INTERFACE_INCLUDE_DIRECTORIES)

                if(NETDATA_PROTOBUF_CFLAGS_OTHER STREQUAL NETDATA_PROTOBUF_CFLAGS_OTHER-NOTFOUND)
                        set(NETDATA_PROTOBUF_CFLAGS_OTHER "")
                endif()

                if(NETDATA_PROTOBUF_INCLUDE_DIRS STREQUAL NETDATA_PROTOBUF_INCLUDE_DIRS-NOTFOUND)
                        set(NETDATA_PROTOBUF_INCLUDE_DIRS "")
                endif()
        else()
                set(NETDATA_PROTOBUF_PROTOC_EXECUTABLE ${PROTOBUF_PROTOC_EXECUTABLE})
                set(NETDATA_PROTOBUF_CFLAGS_OTHER ${PROTOBUF_CFLAGS_OTHER})
                set(NETDATA_PROTOBUF_INCLUDE_DIRS ${PROTOBUF_INCLUDE_DIRS})
                set(NETDATA_PROTOBUF_LIBS ${PROTOBUF_LIBRARIES})
        endif()

        set(ENABLE_PROTOBUF True)
        set(HAVE_PROTOBUF True)
endmacro()

# Helper function to compile protocol definitions into C++ code.
function(netdata_protoc_generate_cpp INC_DIR OUT_DIR SRCS HDRS)
        if(NOT ARGN)
                message(SEND_ERROR "Error: protoc_generate_cpp() called without any proto files")
                return()
        endif()

        set(${INC_DIR})
        set(${OUT_DIR})
        set(${SRCS})
        set(${HDRS})

        foreach(FIL ${ARGN})
                get_filename_component(ABS_FIL ${FIL} ABSOLUTE)
                get_filename_component(DIR ${ABS_FIL} DIRECTORY)
                get_filename_component(FIL_WE ${FIL} NAME_WE)

                set(GENERATED_PB_CC "${DIR}/${FIL_WE}.pb.cc")
                list(APPEND ${SRCS} ${GENERATED_PB_CC})

                set(GENERATED_PB_H "${DIR}/${FIL_WE}.pb.h")
                list(APPEND ${HDRS} ${GENERATED_PB_H})

                list(APPEND _PROTOC_INCLUDE_DIRS ${INC_DIR})

                if(ENABLE_BUNDLED_PROTOBUF)
                        list(APPEND _PROTOC_INCLUDE_DIRS ${CMAKE_BINARY_DIR}/_deps/protobuf-src/src/)
                endif()

                add_custom_command(OUTPUT ${GENERATED_PB_CC} ${GENERATED_PB_H}
                                   COMMAND ${NETDATA_PROTOBUF_PROTOC_EXECUTABLE}
                                   ARGS "-I$<JOIN:${_PROTOC_INCLUDE_DIRS},;-I>" --cpp_out=${OUT_DIR} ${ABS_FIL}
                                   DEPENDS ${ABS_FIL} ${NETDATA_PROTOBUF_PROTOC_EXECUTABLE}
                                   COMMENT "Running C++ protocol buffer compiler on ${FIL}"
                                   COMMAND_EXPAND_LISTS)
        endforeach()

        set_source_files_properties(${${SRCS}} ${${HDRS}} PROPERTIES GENERATED TRUE)
        set_source_files_properties(${${SRCS}} ${${HDRS}} PROPERTIES COMPILE_OPTIONS -Wno-deprecated-declarations)

        set(${SRCS} ${${SRCS}} PARENT_SCOPE)
        set(${HDRS} ${${HDRS}} PARENT_SCOPE)
endfunction()

# Add protobuf to a specified target.
function(netdata_add_protobuf _target)
        target_compile_definitions(${_target} PRIVATE ${NETDATA_PROTOBUF_CFLAGS_OTHER})
        target_include_directories(${_target} PRIVATE ${NETDATA_PROTOBUF_INCLUDE_DIRS})
        target_link_libraries(${_target} PRIVATE ${NETDATA_PROTOBUF_LIBS})
endfunction()
