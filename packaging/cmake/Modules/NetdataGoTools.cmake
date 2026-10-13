# SPDX-License-Identifier: GPL-3.0-or-later
# Shared Go build helpers: the GO_LDFLAGS that bake the version and the
# install paths into the binaries' buildinfo package, add_go_target(), and
# find_min_go_version().
#
# The paths are read at include time, so this module - via NetdataGo or
# NetdataAlarmNotifyGo - must come after NetdataInstallLayout.cmake; the root
# file orders both paths that way.

include_guard()

if(CMAKE_BUILD_TYPE STREQUAL Debug)
    set(GO_LDFLAGS "")
else()
    set(GO_LDFLAGS "-w -s")
endif()

set(BUILDINFO_PKG "github.com/netdata/netdata/go/plugins/pkg/buildinfo")

set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.Version=${NETDATA_VERSION_STRING}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.NetdataBinDir=${NETDATA_BIN_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.PluginsDir=${PLUGINS_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.UserConfigDir=${CONFIG_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.StockConfigDir=${LIBCONFIG_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.StockDataDir=${STOCK_DATA_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.CacheDir=${CACHE_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.VarLibDir=${VARLIB_DIR}")
set(GO_LDFLAGS "${GO_LDFLAGS} -X ${BUILDINFO_PKG}.LogDir=${LOG_DIR}")

# add_go_target: Add a new target that needs to be built using the Go toolchain.
#
# Takes four arguments: the target name, the output artifact name (produced
# under the build directory), the source tree for the Go module, and the
# sub-directory of that source tree to pass to go build.
#
# The dependency list is every *.go file under the source tree plus its
# go.mod and go.sum, so the artifact rebuilds when any of them changes.
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
# Takes one argument, specifying a source tree to scan for go.mod files, and
# sets MIN_GO_VERSION to the highest go directive found: each module states
# its own minimum, and the tree as a whole needs the strictest of them.
function(find_min_go_version src_tree)
    message(STATUS "Determining minimum required version of Go for this build")

    file(GLOB_RECURSE go_mod_files ${src_tree}/go.mod)

    set(result 1.0)

    foreach(f IN ITEMS ${go_mod_files})
        message(VERBOSE "Checking Go version specified in ${f}")
        file(STRINGS "${f}" match_line REGEX "^go .*$")

        if(match_line)
            list(GET match_line 0 go_mod_version)
            string(REGEX MATCH "([0-9]+\\.[0-9]+(\\.[0-9]+)?)" go_mod_version "${go_mod_version}")

            if(go_mod_version VERSION_GREATER result)
                set(result "${go_mod_version}")
            endif()
        endif()
    endforeach()

    message(STATUS "Minimum required Go version determined to be ${result}")
    set(MIN_GO_VERSION "${result}" PARENT_SCOPE)
endfunction()
