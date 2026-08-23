# SPDX-License-Identifier: GPL-3.0-or-later
# libnetdata: the foundation library, plus the two static libraries it is
# built on - judy and netipc.
#
# Unlike the packaging/cmake/Modules/*.cmake files this directory is
# add_subdirectory()d, so CMAKE_CURRENT_SOURCE_DIR is src/libnetdata and every
# relative path below is relative to it. The modules' rule is inverted here:
# SourceLists.cmake is included through CMAKE_CURRENT_LIST_DIR, and a relative
# path must NOT carry a src/libnetdata/ prefix. An absolute
# ${CMAKE_SOURCE_DIR}/src/libnetdata/... still spells the prefix out and is
# left alone. The vendored src/collectors/debugfs.plugin/libsensors directory
# is the prior art for the relative convention; it spells no absolute out.
#
# Three targets rather than one. netipc is self-contained enough to justify a
# nested directory of its own and deliberately does not get one: libnetdata is
# the boundary the project recognises, and nested boundaries would invent a
# distinction nobody makes.
#
# A subdirectory has its own scope, so no VARIABLE set below is visible to the
# root unless the export block at the end of this file hands it back. That
# applies to the source lists and the WEVT_* paths as much as to the six
# exported names: a future root consumer of any of them reads an empty value and
# gets no error.
#
# Targets are the opposite - they are global, and the root relies on it.
# NetdataPluginNetworkViewer.cmake, included well after this directory, adds
# -ffat-lto-objects to judy, libnetdata and netipc by name through an
# if(TARGET ...) guard. That guard is why the failure would be silent if these
# ever stopped being visible, and its arm only runs on 32-bit GCC with LTO, so
# no ordinary build exercises it.

include(${CMAKE_CURRENT_LIST_DIR}/SourceLists.cmake)

#
# compose LIBNETDATA_FILES
#
if(OS_WINDOWS)
    list(APPEND LIBNETDATA_FILES ${LIBNETDATA_WIN_FILES})
endif()

if(OS_LINUX OR OS_WINDOWS)
    list(APPEND LIBNETDATA_FILES
            ${LIBNETDATA_NETIPC_FILES}
    )
endif()

if(NOT OS_FREEBSD)
    list(APPEND LIBNETDATA_FILES ${LIBNETDATA_STACKTRACE_FILES})
endif()

list(APPEND LIBNETDATA_FILES ${INICFG_FILES})
list(APPEND LIBNETDATA_FILES ${LIBNETDATA_CONFIG_FILES})

#
# build libjudy
#

add_library(judy STATIC ${LIBJUDY_FILES})

target_compile_options(judy PRIVATE
        -Wno-all -Wno-extra
        -Wno-shadow
        -Wformat
)

target_compile_definitions(judy PRIVATE
        JUDYL
        $<$<EQUAL:${CMAKE_SIZEOF_VOID_P},8>:JU_64BIT>
)

target_include_directories(judy PUBLIC
        libjudy/vendored
        libjudy/vendored/JudyCommon
)

set_source_files_properties(${LIBJUDY_PREV_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYPREV")
set_source_files_properties(${LIBJUDY_NEXT_FILES} PROPERTIES COMPILE_OPTIONS "-DJUDYNEXT")
set_source_files_properties(libjudy/vendored/JudyL/j__udyLGet.c PROPERTIES COMPILE_OPTIONS "-DJUDYGETINLINE")
set_source_files_properties(libjudy/vendored/JudyL/JudyLByCount.c PROPERTIES COMPILE_OPTIONS "-DNOSMARTJBB -DNOSMARTJBU -DNOSMARTJLB")
# A fourth call, on a bare JudyLTables.c, was dropped in e72c68bca0 rather than
# moved: the bare name resolved against the repository root and matched nothing,
# and the -I it added pointed at a directory that does not exist.

#
# build netipc (standalone IPC library, no Netdata deps)
#

if(OS_LINUX)
    set(NETIPC_FILES
            ${NETIPC_PROTOCOL_FILES}
            ${NETIPC_POSIX_TRANSPORT_FILES}
            ${NETIPC_SERVICE_COMMON_FILES}
            ${NETIPC_POSIX_SERVICE_FILES}
    )

    add_library(netipc STATIC ${NETIPC_FILES})
    target_include_directories(netipc PUBLIC ${CMAKE_SOURCE_DIR}/src/libnetdata/netipc/include)

    if(HAVE_LIBRT)
        target_link_libraries(netipc PUBLIC pthread rt)
    else()
        target_link_libraries(netipc PUBLIC pthread)
    endif()

    set_target_properties(netipc PROPERTIES C_STANDARD 11 C_STANDARD_REQUIRED ON)
elseif(OS_WINDOWS)
    set(NETIPC_FILES
            ${NETIPC_PROTOCOL_FILES}
            ${NETIPC_WINDOWS_TRANSPORT_FILES}
            ${NETIPC_SERVICE_COMMON_FILES}
            ${NETIPC_WINDOWS_SERVICE_FILES}
    )

    add_library(netipc STATIC ${NETIPC_FILES})
    target_include_directories(netipc PUBLIC ${CMAKE_SOURCE_DIR}/src/libnetdata/netipc/include)
    set_target_properties(netipc PROPERTIES C_STANDARD 11 C_STANDARD_REQUIRED ON)
endif()

#
# build libnetdata
#

add_library(libnetdata STATIC ${LIBNETDATA_FILES})

include(CheckCCompilerFlag)

if(CMAKE_SYSTEM_PROCESSOR MATCHES "(arm64)|(aarch64)|(ARM64)|(AARCH64)")
    check_c_compiler_flag("-mno-outline-atomics" HAVE_C_MNO_OUTLINE_ATOMICS)
    if(HAVE_C_MNO_OUTLINE_ATOMICS)
        # This source is reached from raw signal handlers and cannot call atomic runtime helpers.
        set_source_files_properties(exit/exit_initiated.c PROPERTIES
                                    COMPILE_OPTIONS "-mno-outline-atomics")
    endif()
endif()

set_source_files_properties(gorilla/gorilla.cc PROPERTIES COMPILE_OPTIONS "-Wno-attributes")

target_include_directories(libnetdata BEFORE PUBLIC ${CMAKE_BINARY_DIR} ${CMAKE_SOURCE_DIR}/src)

target_link_libraries(libnetdata PUBLIC
        "$<$<NOT:$<BOOL:${HAVE_BUILTIN_ATOMICS}>>:atomic>"
        "$<$<OR:$<BOOL:${OS_LINUX}>,$<BOOL:${OS_FREEBSD}>>:pthread>"
        "$<$<BOOL:${HAVE_LIBRT}>:rt>"
        "$<$<BOOL:${OS_WINDOWS}>:kernel32;advapi32;winmm;rpcrt4;wevtapi;ole32;oleaut32;wbemuuid;sensorsapi;iphlpapi>"
        "$<$<BOOL:${LINK_LIBM}>:m>"
        "${SYSTEMD_LDFLAGS}")

if(OS_LINUX OR OS_WINDOWS)
    target_link_libraries(libnetdata PUBLIC netipc)
endif()

if(HAVE_LIBBACKTRACE)
    netdata_add_libbacktrace_to_target(libnetdata)
endif()

netdata_add_sqlite3_to_target(libnetdata)

if(OS_WINDOWS)
        set(HAVE_ETW True)
        set(HAVE_WEL True)

        # Output the results for debugging purposes
        message(STATUS "Have Event Tracing for Windows (ETW): ${HAVE_ETW}")
        message(STATUS "Have Windows Event Log (WEL): ${HAVE_WEL}")

        if(HAVE_WEL OR HAVE_ETW)
                # Define the source and generated file paths
                set(WEVT_GEN_SRC_H_FILE "${CMAKE_SOURCE_DIR}/src/libnetdata/log/nd_log-to-windows-common.h")
                set(WEVT_GEN_SRC_C_FILE "${CMAKE_SOURCE_DIR}/src/libnetdata/log/wevt_netdata_mc_generate.c")
                set(WEVT_GEN_BIN_FILE "${CMAKE_BINARY_DIR}/wevt_netdata_mc_generate")

                set(WEVT_BUILD_SCRIPT "${CMAKE_SOURCE_DIR}/src/libnetdata/log/wevt_netdata_compile.sh")

                set(WEVT_MC_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.mc")
                set(WEVT_MAN_FILE "${CMAKE_BINARY_DIR}/wevt_netdata_manifest.xml")
                set(WEVT_RC_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.rc")
                set(WEVT_MC_H_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.h")
                set(WEVT_MAN_H_FILE "${CMAKE_BINARY_DIR}/wevt_netdata_manifest.h")
                set(WEVT_RES_OBJECT "${CMAKE_BINARY_DIR}/wevt_netdata_res.o")

                set(WEVT_DLL_FILE "${CMAKE_BINARY_DIR}/wevt_netdata.dll")
                set(WEVT_ETW_INSTALL_SCRIPT "${CMAKE_SOURCE_DIR}/src/libnetdata/log/wevt_netdata_install.bat")

                # we compile ${WEVT_GEN_BIN_FILE}, which generates the manifest, the .mc,
                # and the headers required for compiling libnetdata/logs
                #
                # WORKING_DIRECTORY is pinned on both arms below. A custom command
                # with none runs in the build directory matching the current source
                # directory, which this move changed from ${CMAKE_BINARY_DIR} to
                # ${CMAKE_BINARY_DIR}/src/libnetdata. Neither arm actually depends
                # on it: every argument is absolute, and windres resolves the names
                # inside a .rc against the .rc's own directory rather than the
                # working directory - tested against real binutils, with a negative
                # control. The pin restores the pre-move value exactly, so it costs
                # nothing and keeps a later cwd-relative edit here from going wrong
                # on a platform none of us can build.

                if(HAVE_ETW)
                        # ETW method also supports WEL
                        # but it requires Microsoft tools mc, rc, and link
                        add_custom_command(
                                OUTPUT "${WEVT_MC_H_FILE}" "${WEVT_MAN_H_FILE}" "${WEVT_DLL_FILE}"
                                COMMAND "${CMAKE_C_COMPILER}" -o "${WEVT_GEN_BIN_FILE}" "${WEVT_GEN_SRC_C_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" >"${WEVT_MC_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" --manifest >"${WEVT_MAN_FILE}"
                                COMMAND "${WEVT_BUILD_SCRIPT}" "${CMAKE_SOURCE_DIR}/src/libnetdata/log" "${CMAKE_BINARY_DIR}"
                                WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
                                DEPENDS "${WEVT_GEN_SRC_C_FILE}" "${WEVT_GEN_SRC_H_FILE}"
                                COMMENT "Compiling ${WEVT_MC_FILE} to generate ${WEVT_MC_H_FILE} and ${WEVT_DLL_FILE}"
                        )
                else()
                        # WEL method can be built with windmc, windres and the normal linker
                        add_custom_command(
                                OUTPUT "${WEVT_MC_H_FILE}" "${WEVT_DLL_FILE}"
                                COMMAND "${CMAKE_C_COMPILER}" -o "${WEVT_GEN_BIN_FILE}" "${WEVT_GEN_SRC_C_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" >"${WEVT_MC_FILE}"
                                COMMAND "${WEVT_GEN_BIN_FILE}" --manifest >"${WEVT_MAN_FILE}"
                                COMMAND windmc -r "${CMAKE_BINARY_DIR}" -h "${CMAKE_BINARY_DIR}" ${WEVT_MC_FILE}
                                COMMAND echo "1 2004" "wevt_netdata_manifest.xml" >> "${WEVT_RC_FILE}"
                                COMMAND windres ${WEVT_RC_FILE} -o ${WEVT_RES_OBJECT}
                                COMMAND ${CMAKE_LINKER} -dll --entry 0 -nostdlib -o ${WEVT_DLL_FILE} ${WEVT_RES_OBJECT}
                                WORKING_DIRECTORY "${CMAKE_BINARY_DIR}"
                                DEPENDS "${WEVT_GEN_SRC_C_FILE}" "${WEVT_GEN_SRC_H_FILE}"
                                COMMENT "Compiling ${WEVT_MC_FILE} to generate ${WEVT_MC_H_FILE} and ${WEVT_DLL_FILE}"
                        )
                endif()

                # Create a custom target for the DLL
                add_custom_target(wevt_netdata ALL DEPENDS ${WEVT_DLL_FILE})

                set_source_files_properties(log/nd_log-to-windows-events.c PROPERTIES
                        OBJECT_DEPENDS "${WEVT_MC_H_FILE}")

                if(HAVE_ETW)
                        set_source_files_properties(log/nd_log-to-windows-events.c PROPERTIES
                                OBJECT_DEPENDS "${WEVT_MAN_H_FILE}")

                        install(FILES "${WEVT_DLL_FILE}" "${WEVT_MAN_FILE}" "${WEVT_ETW_INSTALL_SCRIPT}"
                                COMPONENT wevt_netdata_dll
                                DESTINATION "${BINDIR}")
                else()
                        # do not install the manifest in this case
                        # the nsi installer will skip registering the ETW publisher
                        install(FILES "${WEVT_DLL_FILE}"
                                COMPONENT wevt_netdata_dll
                                DESTINATION "${BINDIR}")
                endif()
        endif()
endif()

# judy
target_link_libraries(libnetdata PUBLIC judy)

netdata_add_jsonc_to_target(libnetdata)

netdata_add_libyaml_to_target(libnetdata)

# libunwind
if(ENABLE_LIBUNWIND)
    set(HAVE_LIBUNWIND On)

    if(STATIC_BUILD)
      set(unwind_prefix /libunwind-static/lib/libunwind-)
      set(suffix ${CMAKE_STATIC_LIBRARY_SUFFIX})
    else()
      set(unwind_prefix -lunwind-)
      set(suffix "")
    endif()

    if(CMAKE_SYSTEM_PROCESSOR MATCHES "(x86_64)|(amd64)")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}x86_64${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "^i?.86")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}x86${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "(arm64)|(aarch64)")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}aarch64${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "arm")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}arm${suffix})
    elseif(CMAKE_SYSTEM_PROCESSOR MATCHES "p(ower)?pc64")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND ${unwind_prefix}ppc64${suffix})
    else()
      message(WARNING "Unknown architecture ${CMAKE_SYSTEM_PROCESSOR} for libunwind. Stack traces may not work.")
      target_link_libraries(libnetdata PUBLIC PkgConfig::LIBUNWIND)
    endif()
endif()

# zlib
target_include_directories(libnetdata BEFORE PUBLIC ${ZLIB_INCLUDE_DIRS})

# FindZLIB reports an imported target; pkg-config reports flags. Both arms are
# reached, so both spellings of "link zlib" have to stay.
if(OS_MACOS)
        target_link_libraries(libnetdata PUBLIC ZLIB::ZLIB)
else()
        target_compile_options(libnetdata PUBLIC ${ZLIB_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${ZLIB_LDFLAGS})
endif()

# lz4
set(ENABLE_LZ4 On)

target_include_directories(libnetdata BEFORE PUBLIC ${LIBLZ4_INCLUDE_DIRS})
target_compile_options(libnetdata PUBLIC ${LIBLZ4_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${LIBLZ4_LDFLAGS})

# zstd
if(LIBZSTD_FOUND)
        set(ENABLE_ZSTD On)
        target_include_directories(libnetdata BEFORE PUBLIC ${LIBZSTD_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${LIBZSTD_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBZSTD_LDFLAGS})
endif()

# brotli
if(LIBBROTLI_FOUND)
        set(ENABLE_BROTLI On)
        target_include_directories(libnetdata PUBLIC ${LIBBROTLI_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${LIBBROTLI_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${LIBBROTLI_LDFLAGS})
endif()

# uuid
if(OS_MACOS OR OS_WINDOWS)
        # UUID functionality is part of the system libraries here, so no extra
        # stuff needed.
else()
        target_include_directories(libnetdata BEFORE PUBLIC ${UUID_INCLUDE_DIRS})
        target_compile_options(libnetdata PUBLIC ${UUID_CFLAGS_OTHER})
        target_link_libraries(libnetdata PUBLIC ${UUID_LDFLAGS})
endif()

# uv
target_include_directories(libnetdata BEFORE PUBLIC ${LIBUV_INCLUDE_DIRS})
target_compile_options(libnetdata PUBLIC ${LIBUV_CFLAGS_OTHER})
target_link_libraries(libnetdata PUBLIC ${LIBUV_LDFLAGS})

# crypto
target_link_libraries(libnetdata PUBLIC PkgConfig::CRYPTO)

# openssl
target_link_libraries(libnetdata PUBLIC PkgConfig::TLS)

#
# spawn-server test binary
#
# No install rule; it exists to be run from the build tree by
# tests/run-unit-tests.sh, which invokes it as <build>/spawn-tester. A
# subdirectory would otherwise put it in <build>/src/libnetdata, so the
# output directory is pinned to keep that path working.
#
add_executable(spawn-tester spawn_server/spawn-tester.c)
target_link_libraries(spawn-tester libnetdata)
set_target_properties(spawn-tester PROPERTIES
                      RUNTIME_OUTPUT_DIRECTORY "${CMAKE_BINARY_DIR}")

#
# exported to the parent scope
#
# A subdirectory gets its own variable scope, so everything the root still
# reads has to be handed back explicitly. Every consumer is named below, so
# this list can be checked rather than trusted - a seventh consumer added
# elsewhere fails silently unless it is added here too.
#
# All six are read by packaging/cmake/config.cmake.h.in, each as a
# #cmakedefine of the same name, and reach the build through config.h, which
# NetdataSystemFiles.cmake generates from the root after this directory. A
# missing export costs no error: config.h simply emits #undef and the feature
# disappears.
#
set(ENABLE_LZ4     "${ENABLE_LZ4}"     PARENT_SCOPE)
set(ENABLE_ZSTD    "${ENABLE_ZSTD}"    PARENT_SCOPE)
set(ENABLE_BROTLI  "${ENABLE_BROTLI}"  PARENT_SCOPE)
set(HAVE_WEL       "${HAVE_WEL}"       PARENT_SCOPE)
set(HAVE_ETW       "${HAVE_ETW}"       PARENT_SCOPE)
set(HAVE_LIBUNWIND "${HAVE_LIBUNWIND}" PARENT_SCOPE)
