# Macros to assist in building Go components
#
# Copyright (c) 2024 Netdata Inc
#
# SPDX-License-Identifier: GPL

if(CMAKE_BUILD_TYPE STREQUAL Debug OR CMAKE_BUILD_TYPE STREQUAL RelWithDebInfo)
    set(GO_LDFLAGS "-X main.version=${NETDATA_VERSION}")
else()
    set(GO_LDFLAGS "-w -s -X main.version=${NETDATA_VERSION}")
endif()

# add_go_target: Add a new target that needs to be built using the Go toolchain.
#
# Takes four arguments, the target name, the output artifact name, the
# source tree for the Go module, and the sub-directory of that source tree
# to pass to `go build`.
#
# The target itself will invoke `go build` in the specified source tree,
# using the `-o` option to produce the final output artifact, and passing
# the requested sub-directory as the final argument.
#
# This will also automatically construct the dependency list for the
# target by finding all Go source files under the specified source tree
# and then appending the go.mod and go.sum files from the root of the
# source tree.
macro(add_go_target target output build_src build_dir)
    file(GLOB_RECURSE ${target}_DEPS CONFIGURE_DEPENDS "${build_src}/*.go")
    list(APPEND ${target}_DEPS
        "${build_src}/go.mod"
        "${build_src}/go.sum"
    )

    add_custom_command(
        OUTPUT ${output}
        COMMAND "${CMAKE_COMMAND}" -E env CGO_ENABLED=0 "${GO_EXECUTABLE}" build -buildvcs=false -ldflags "${GO_LDFLAGS}" -o "${CMAKE_BINARY_DIR}/${output}" "./${build_dir}"
        DEPENDS ${${target}_DEPS}
        COMMENT "Building Go component ${output}"
        WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/${build_src}"
        VERBATIM
    )
    add_custom_target(
        ${target} ALL
        DEPENDS ${output}
    )
endmacro()
