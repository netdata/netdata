# SPDX-License-Identifier: GPL-3.0-or-later
# Macros and functions to assist in working with Go

if(CMAKE_BUILD_TYPE STREQUAL Debug)
    set(GO_LDFLAGS "-X github.com/netdata/netdata/go/plugins/pkg/buildinfo.Version=${NETDATA_VERSION_STRING}")
else()
    set(GO_LDFLAGS "-w -s -X github.com/netdata/netdata/go/plugins/pkg/buildinfo.Version=${NETDATA_VERSION_STRING}")
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
        COMMAND "${CMAKE_COMMAND}" -E env GOROOT=${GO_ROOT} CGO_ENABLED=0 GOPROXY=https://proxy.golang.org,direct "${GO_EXECUTABLE}" build -buildvcs=false -ldflags "${GO_LDFLAGS}" -o "${CMAKE_BINARY_DIR}/${output}" "./${build_dir}"
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

# find_min_go_version: Determine the minimum Go version based on go.mod files
#
# Takes one argument, specifying a source tree to scan for go.mod files.
#
# All files found will be checked for a `go` directive, and the
# MIN_GO_VERSION variable will be set to the highest version
# number found among these directives.
#
# Only works on UNIX-like systems, because it has to process the go.mod
# files in ways that CMake can't do on it's own.
function(find_min_go_version src_tree)
    message(STATUS "Determining minimum required version of Go for this build")

    file(GLOB_RECURSE go_mod_files ${src_tree}/go.mod)

    set(result 1.0)

    foreach(f IN ITEMS ${go_mod_files})
        message(VERBOSE "Checking Go version specified in ${f}")
        execute_process(
            COMMAND grep -E "^go .*$" ${f}
            COMMAND cut -f 2 -d " "
            RESULT_VARIABLE version_check_result
            OUTPUT_VARIABLE go_mod_version
        )

        if(version_check_result EQUAL 0)
            string(REGEX MATCH "([0-9]+\\.[0-9]+(\\.[0-9]+)?)" go_mod_version "${go_mod_version}")

            if(go_mod_version VERSION_GREATER result)
                set(result "${go_mod_version}")
            endif()
        endif()
    endforeach()

    message(STATUS "Minimum required Go version determined to be ${result}")
    set(MIN_GO_VERSION "${result}" PARENT_SCOPE)
endfunction()
