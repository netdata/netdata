# SPDX-License-Identifier: GPL-3.0-or-later
# Macros and functions for handling of Protobuf

# Prepare a vendored copy of Protobuf for use with Netdata.
function(netdata_bundle_protobuf)
        include(FetchContent)
        include(NetdataFetchContentExtra)

        set(PROTOBUF_TAG f0dc78d7e6e331b8c6bb2d5283e06aa26883ca7c) # v21.12
        set(NEED_ABSL False)

        if(CMAKE_CXX_STANDARD GREATER_EQUAL 14)
                set(PROTOBUF_TAG 4a2aef570deb2bfb8927426558701e8bfc26f2a4) # v25.3
                set(NEED_ABSL True)
                set(ABSL_TAG 2f9e432cce407ce0ae50676696666f33a77d42ac) # 20240116.1
        endif()

        set(FETCHCONTENT_TRY_FIND_PACKAGE_MODE NEVER)

        string(REPLACE "-fsanitize=address" "" CMAKE_C_FLAGS "${CMAKE_C_FLAGS}")
        string(REPLACE "-fsanitize=address" "" CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS}")

        # ignore debhelper
        set(FETCHCONTENT_FULLY_DISCONNECTED Off)

        if(NEED_ABSL)
                set(ABSL_PROPAGATE_CXX_STD On)
                set(ABSL_ENABLE_INSTALL Off)
                set(BUILD_SHARED_LIBS Off)
                set(ABSL_BUILD_TESTING Off)
                set(absl_SOURCE_DIR "${CMAKE_BINARY_DIR}/_deps/absl-src")

                message(STATUS "Preparing bundled Abseil (required by bundled Protobuf)")
                find_program(PATCH patch REQUIRED)
                if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
                    FetchContent_Declare(absl
                            GIT_REPOSITORY https://github.com/abseil/abseil-cpp
                            GIT_TAG ${ABSL_TAG}
                            SOURCE_DIR ${absl_SOURCE_DIR}
                            PATCH_COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/patches/apply-patches.sh
                                        ${absl_SOURCE_DIR}
                                        ${CMAKE_SOURCE_DIR}/packaging/cmake/patches/abseil
                            CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
                            EXCLUDE_FROM_ALL
                    )
                else()
                    FetchContent_Declare(absl
                            GIT_REPOSITORY https://github.com/abseil/abseil-cpp
                            GIT_TAG ${ABSL_TAG}
                            SOURCE_DIR ${absl_SOURCE_DIR}
                            PATCH_COMMAND ${CMAKE_SOURCE_DIR}/packaging/cmake/patches/apply-patches.sh
                                        ${absl_SOURCE_DIR}
                                        ${CMAKE_SOURCE_DIR}/packaging/cmake/patches/abseil
                            CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
                    )
                endif()
                FetchContent_MakeAvailable_NoInstall(absl)
                message(STATUS "Finished preparing bundled Abseil")
        endif()

        set(protobuf_INSTALL Off)
        set(protobuf_BUILD_LIBPROTOC Off)
        set(protobuf_BUILD_TESTS Off)
        set(protobuf_BUILD_SHARED_LIBS Off)
        set(protobuf_repo https://github.com/protocolbuffers/protobuf)

        message(STATUS "Preparing bundled Protobuf")
        if(CMAKE_VERSION VERSION_GREATER_EQUAL 3.28)
                FetchContent_Declare(protobuf
                        GIT_REPOSITORY ${protobuf_repo}
                        GIT_TAG ${PROTOBUF_TAG}
                        CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
                        EXCLUDE_FROM_ALL
                )
        else()
                FetchContent_Declare(protobuf
                        GIT_REPOSITORY ${protobuf_repo}
                        GIT_TAG ${PROTOBUF_TAG}
                        CMAKE_ARGS ${NETDATA_CMAKE_PROPAGATE_TOOLCHAIN_ARGS}
                )
        endif()
        FetchContent_MakeAvailable_NoInstall(protobuf)
        message(STATUS "Finished preparing bundled Protobuf.")

        set(ENABLE_BUNDLED_PROTOBUF True PARENT_SCOPE)
endfunction()

# Handle detection of Protobuf
macro(netdata_detect_protobuf)
        if(OS_WINDOWS)
                set(PROTOBUF_PROTOC_EXECUTABLE "$ENV{PROTOBUF_PROTOC_EXECUTABLE}")
                if(NOT PROTOBUF_PROTOC_EXECUTABLE)
                        set(PROTOBUF_PROTOC_EXECUTABLE "/bin/protoc")
                endif()
                set(PROTOBUF_CFLAGS_OTHER "")
                set(PROTOBUF_INCLUDE_DIRS "")
                set(PROTOBUF_LIBRARIES "-lprotobuf")

                set(ENABLE_PROTOBUF True)
                set(HAVE_PROTOBUF True)
        else()
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
                  get_property(IMPORTED_SET TARGET protobuf::libprotobuf PROPERTY IMPORTED_LOCATION SET)
                  get_property(ALIASED TARGET protobuf::libprotobuf PROPERTY ALIASED_TARGET)

                  if(ALIASED STREQUAL "" AND NOT IMPORTED_SET)
                    set_property(TARGET protobuf::libprotobuf PROPERTY IMPORTED_LOCATION "${Protobuf_LIBRARY}")
                  endif()

                  if(NOT Protobuf_PROTOC_EXECUTABLE AND TARGET protobuf::protoc)
                    set(Protobuf_PROTOC_EXECUTABLE protobuf::protoc)
                  endif()

                  # It is technically possible that this may still not
                  # be set by this point, so we need to check it and
                  # fail noisily if it isn't because the build won't
                  # work without it.
                  if(NOT Protobuf_PROTOC_EXECUTABLE)
                    message(FATAL_ERROR "Could not determine the location of the protobuf compiler for the detected version of protobuf.")
                  endif()

                  set(PROTOBUF_PROTOC_EXECUTABLE ${Protobuf_PROTOC_EXECUTABLE})
                  set(PROTOBUF_LIBRARIES protobuf::libprotobuf)
                endif()

                set(ENABLE_PROTOBUF True)
                set(HAVE_PROTOBUF True)
        endif()
endmacro()


# Helper function to compile protocol definitions into C++ code.
function(netdata_protoc_generate_cpp PROTO_ROOT_DIR OUTPUT_ROOT_DIR GENERATED_SOURCES GENERATED_HEADERS)
    if(NOT ARGN)
        message(SEND_ERROR "Error: netdata_protoc_generate_cpp() called without any proto files")
        return()
    endif()

    # Initialize output variables
    set(output_sources)
    set(output_headers)

    # Setup include paths for protoc
    set(protoc_include_paths ${PROTO_ROOT_DIR})
    if(ENABLE_BUNDLED_PROTOBUF)
        list(APPEND protoc_include_paths ${CMAKE_BINARY_DIR}/_deps/protobuf-src/src/)
    endif()

    # Process each proto file
    foreach(proto_file ${ARGN})
        # Get absolute paths and component parts
        get_filename_component(proto_file_abs_path ${proto_file} ABSOLUTE)
        get_filename_component(proto_file_name_no_ext ${proto_file} NAME_WE)
        get_filename_component(proto_file_dir ${proto_file} DIRECTORY)

        # Calculate relative output path to maintain directory structure
        get_filename_component(proto_root_abs_path ${PROTO_ROOT_DIR} ABSOLUTE)
        get_filename_component(proto_dir_abs_path ${proto_file_dir} ABSOLUTE)
        file(RELATIVE_PATH proto_relative_path ${proto_root_abs_path} ${proto_dir_abs_path})

        # Construct output file paths
        set(output_dir "${OUTPUT_ROOT_DIR}/${proto_relative_path}")
        set(generated_source "${output_dir}/${proto_file_name_no_ext}.pb.cc")
        set(generated_header "${output_dir}/${proto_file_name_no_ext}.pb.h")

        # Add to output lists
        list(APPEND output_sources "${generated_source}")
        list(APPEND output_headers "${generated_header}")

        # Create custom command to generate the protobuf files
        add_custom_command(
            OUTPUT "${generated_source}" "${generated_header}"
            COMMAND ${CMAKE_COMMAND} -E make_directory "${output_dir}"
            COMMAND ${PROTOBUF_PROTOC_EXECUTABLE}
            ARGS "-I$<JOIN:${protoc_include_paths},;-I>" 
                 --cpp_out=${OUTPUT_ROOT_DIR} 
                 ${proto_file_abs_path}
            DEPENDS ${proto_file_abs_path} ${PROTOBUF_PROTOC_EXECUTABLE}
            COMMENT "Generating C++ protocol buffer files from ${proto_file}"
            COMMAND_EXPAND_LISTS
            VERBATIM
        )
    endforeach()

    # Mark generated files with proper properties
    set_source_files_properties(
        ${output_sources} ${output_headers}
        PROPERTIES 
            GENERATED TRUE
            COMPILE_OPTIONS -Wno-deprecated-declarations
    )

    # Set output variables in parent scope
    set(${GENERATED_SOURCES} ${output_sources} PARENT_SCOPE)
    set(${GENERATED_HEADERS} ${output_headers} PARENT_SCOPE)
endfunction()

# Add protobuf to a specified target.
function(netdata_add_protobuf _target)
        if(ENABLE_BUNDLED_PROTOBUF)
            target_include_directories(${_target} BEFORE PRIVATE ${PROTOBUF_INCLUDE_DIRS})
        else()
            target_include_directories(${_target} PRIVATE ${PROTOBUF_INCLUDE_DIRS})
        endif()

        target_compile_options(${_target} PRIVATE ${PROTOBUF_CFLAGS_OTHER})
        target_link_libraries(${_target} PRIVATE ${PROTOBUF_LIBRARIES})
endfunction()
