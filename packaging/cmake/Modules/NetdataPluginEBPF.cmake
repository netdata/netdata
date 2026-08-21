# SPDX-License-Identifier: GPL-3.0-or-later
# ebpf: eBPF-based collection, its Go companion plugin, and the legacy
# programs.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Two source lists moved here from NetdataSourceLists.cmake per D27, in their
# original relative order. INTERCOMMUNICATION_COLLECTORS_FILES has a generic
# name but ebpf.plugin is its only consumer; moving it here asserts that, and
# the assertion was disclosed with D27.
#
# Both stay above the guard, so they are still defined unconditionally.
#
# The GLOB_RECURSE for the Go plugin below uses CMAKE_SOURCE_DIR, not
# CMAKE_CURRENT_LIST_DIR, so include() leaves it resolving exactly as before.

set(INTERCOMMUNICATION_COLLECTORS_FILES
        src/collectors/collectors-ipc/ebpf-ipc.c
        src/collectors/collectors-ipc/ebpf-ipc.h
        )

# ebpf.plugin
set(EBPF_PLUGIN_FILES
        src/collectors/ebpf.plugin/ebpf.c
        src/collectors/ebpf.plugin/ebpf.h
        src/collectors/ebpf.plugin/ebpf_disk.c
        src/collectors/ebpf.plugin/ebpf_disk.h
        src/collectors/ebpf.plugin/ebpf_fd.c
        src/collectors/ebpf.plugin/ebpf_fd.h
        src/collectors/ebpf.plugin/ebpf_hardirq.c
        src/collectors/ebpf.plugin/ebpf_hardirq.h
        src/collectors/ebpf.plugin/ebpf_mdflush.c
        src/collectors/ebpf.plugin/ebpf_mdflush.h
        src/collectors/ebpf.plugin/ebpf_mount.c
        src/collectors/ebpf.plugin/ebpf_mount.h
        src/collectors/ebpf.plugin/ebpf_filesystem.c
        src/collectors/ebpf.plugin/ebpf_filesystem.h
        src/collectors/ebpf.plugin/ebpf_oomkill.c
        src/collectors/ebpf.plugin/ebpf_oomkill.h
        src/collectors/ebpf.plugin/ebpf_process.c
        src/collectors/ebpf.plugin/ebpf_process.h
        src/collectors/ebpf.plugin/ebpf_shm.c
        src/collectors/ebpf.plugin/ebpf_shm.h
        src/collectors/ebpf.plugin/ebpf_softirq.c
        src/collectors/ebpf.plugin/ebpf_softirq.h
        src/collectors/ebpf.plugin/ebpf_sync.c
        src/collectors/ebpf.plugin/ebpf_sync.h
        src/collectors/ebpf.plugin/ebpf_swap.c
        src/collectors/ebpf.plugin/ebpf_swap.h
        src/collectors/ebpf.plugin/ebpf_vfs.c
        src/collectors/ebpf.plugin/ebpf_vfs.h
        src/collectors/ebpf.plugin/ebpf_apps.c
        src/collectors/ebpf.plugin/ebpf_apps.h
        src/collectors/ebpf.plugin/ebpf_cgroup.c
        src/collectors/ebpf.plugin/ebpf_cgroup.h
        src/collectors/ebpf.plugin/ebpf_unittest.c
        src/collectors/ebpf.plugin/ebpf_unittest.h
         src/collectors/ebpf.plugin/libbpf_api/ebpf.c
         src/collectors/ebpf.plugin/libbpf_api/ebpf.h
         src/collectors/ebpf.plugin/libbpf_api/ebpf_library.c
         src/collectors/ebpf.plugin/libbpf_api/ebpf_library.h
 )

if(ENABLE_PLUGIN_EBPF)
    add_executable(ebpf.plugin ${EBPF_PLUGIN_FILES} ${INTERCOMMUNICATION_COLLECTORS_FILES})
    target_link_libraries(ebpf.plugin libnetdata)

    netdata_add_libbpf_to_target(ebpf.plugin)
    netdata_add_ebpf_co_re_to_target(ebpf.plugin)

    install(TARGETS ebpf.plugin
            PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE SETUID
            COMPONENT plugin-ebpf
            DESTINATION ${PLUGINS_DEST})

    # GLOB_RECURSE descends into subdirectories, so netipc/*.go covers
    # protocol/, service/, and transport/ even though no .go files exist at
    # netipc/'s root level.  go.mod and go.sum are listed explicitly so a
    # Go-version bump or new transitive dep in go/plugins triggers a rebuild.
    file(GLOB_RECURSE EBPF_GO_PLUGIN_FILES CONFIGURE_DEPENDS
            "${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin/ebpfgo.plugin/*.go"
            "${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin/ebpfgo.plugin/*.c"
            "${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin/ebpfgo.plugin/*.h"
            "${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin/ebpfgo.plugin/go.mod"
            "${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin/ebpfgo.plugin/go.sum"
            "${CMAKE_SOURCE_DIR}/go.work"
            "${CMAKE_SOURCE_DIR}/src/go/go.mod"
            "${CMAKE_SOURCE_DIR}/src/go/go.sum"
            "${CMAKE_SOURCE_DIR}/src/go/pkg/netipc/*.go"
            "${CMAKE_SOURCE_DIR}/src/go/pkg/netdataapi/*.go")

    set(EBPF_GO_LIBBPF_LIB_DIR lib)
    if(CMAKE_SYSTEM_PROCESSOR MATCHES "(x86_64)|(amd64)")
        set(EBPF_GO_LIBBPF_LIB_DIR lib64)
    endif()

    set(EBPF_GO_CGO_CFLAGS "")
    foreach(include_dir IN LISTS NETDATA_LIBBPF_INCLUDE_DIRECTORIES)
        string(APPEND EBPF_GO_CGO_CFLAGS " -I${include_dir}")
    endforeach()
    string(APPEND EBPF_GO_CGO_CFLAGS " -I${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin")
    string(APPEND EBPF_GO_CGO_CFLAGS " -I${ebpf-co-re_SOURCE_DIR}")
    string(STRIP "${EBPF_GO_CGO_CFLAGS}" EBPF_GO_CGO_CFLAGS)
    set(EBPF_GO_CGO_LDFLAGS
            "-L${libbpf_SOURCE_DIR}/usr/${EBPF_GO_LIBBPF_LIB_DIR} -lbpf -lelf -lz")
    if(HAVE_LIBRT)
        string(APPEND EBPF_GO_CGO_LDFLAGS " -lrt")
    endif()

    if(OS_LINUX AND ENABLE_PLUGIN_EBPF_GO)
        add_custom_command(
                OUTPUT "${CMAKE_BINARY_DIR}/ebpf-go.plugin"
                COMMAND "${CMAKE_COMMAND}" -E env
                        GOROOT=${GO_ROOT}
                        CGO_ENABLED=1
                        CGO_CFLAGS=${EBPF_GO_CGO_CFLAGS}
                        CGO_LDFLAGS=${EBPF_GO_CGO_LDFLAGS}
                        GOPROXY=https://proxy.golang.org,direct
                        "${GO_EXECUTABLE}" build -buildvcs=false
                        -ldflags "-X github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin.netdataRuntimePrefix=${NETDATA_RUNTIME_PREFIX}"
                        -tags netdata_ebpf_libbpf
                        -o "${CMAKE_BINARY_DIR}/ebpf-go.plugin"
                        "."
                DEPENDS ${EBPF_GO_PLUGIN_FILES}
                COMMENT "Building Go eBPF plugin ebpf-go.plugin"
                WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/src/collectors/ebpf.plugin/ebpfgo.plugin"
                VERBATIM)

        add_custom_target(ebpf-go-plugin ALL
                DEPENDS "${CMAKE_BINARY_DIR}/ebpf-go.plugin")

        if(TARGET libbpf)
            add_dependencies(ebpf-go-plugin libbpf)
        endif()
        add_dependencies(ebpf-go-plugin ebpf-co-re)
        install(FILES "${CMAKE_BINARY_DIR}/ebpf-go.plugin"
                PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE SETUID
                COMPONENT plugin-ebpf
                DESTINATION ${PLUGINS_DEST})
    endif()

    install(FILES
            src/collectors/ebpf.plugin/ebpf.d.conf
            COMPONENT plugin-ebpf
            DESTINATION ${LIBCONFIG_DEST})

    install(FILES
            src/collectors/ebpf.plugin/ebpf.d/cachestat.conf
            src/collectors/ebpf.plugin/ebpf.d/dcstat.conf
            src/collectors/ebpf.plugin/ebpf.d/disk.conf
            src/collectors/ebpf.plugin/ebpf.d/ebpf_kernel_reject_list.txt
            src/collectors/ebpf.plugin/ebpf.d/fd.conf
            src/collectors/ebpf.plugin/ebpf.d/filesystem.conf
            src/collectors/ebpf.plugin/ebpf.d/hardirq.conf
            src/collectors/ebpf.plugin/ebpf.d/dns.conf
            src/collectors/ebpf.plugin/ebpf.d/mdflush.conf
            src/collectors/ebpf.plugin/ebpf.d/mount.conf
            src/collectors/ebpf.plugin/ebpf.d/oomkill.conf
            src/collectors/ebpf.plugin/ebpf.d/process.conf
            src/collectors/ebpf.plugin/ebpf.d/shm.conf
            src/collectors/ebpf.plugin/ebpf.d/socket.conf
            src/collectors/ebpf.plugin/ebpf.d/softirq.conf
            src/collectors/ebpf.plugin/ebpf.d/swap.conf
            src/collectors/ebpf.plugin/ebpf.d/sync.conf
            src/collectors/ebpf.plugin/ebpf.d/vfs.conf
            COMPONENT plugin-ebpf
            DESTINATION ${LIBCONFIG_DEST}/ebpf.d)

    netdata_add_deb_copyright(plugin-ebpf netdata-plugin-ebpf)

    if(ENABLE_LEGACY_EBPF_PROGRAMS)
        netdata_install_legacy_ebpf_code()
    endif()
endif()
