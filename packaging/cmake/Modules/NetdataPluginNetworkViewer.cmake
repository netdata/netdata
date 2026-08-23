# SPDX-License-Identifier: GPL-3.0-or-later
# network-viewer: the socket and container topology collector, and its
# two test binaries.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# All five per-OS source lists live here rather than in the shared
# NetdataSourceLists.cmake, so the plugin's whole definition is in one place,
# next to the per-OS list(APPEND) block that assembles them. They sit above
# the guard, so they are defined unconditionally.
#
# The two test targets sit inside the plugin's own guard, so they stay with
# it rather than moving to NetdataTests.cmake.
#
# The i386 fat-LTO workaround below reaches out to judy, libnetdata, netipc
# and json-c. Those targets are created earlier in the root file and include()
# does not change when this runs, so they exist here exactly as before.

include_guard()

# network-viewer: the OS-independent sources
set(NETWORK_VIEWER_COMMON_FILES
        src/collectors/common-cgroups/cgroup-path.c
        src/collectors/common-cgroups/cgroup-path.h
        src/collectors/common-cgroups/cgroup-topology-rules.c
        src/collectors/common-cgroups/cgroup-topology-rules.h
        src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.h
        src/collectors/network-viewer.plugin/network-viewer-topology-containers.c
        src/collectors/network-viewer.plugin/network-viewer-topology-containers.h
        src/collectors/network-viewer.plugin/network-viewer-topology.c
        src/collectors/network-viewer.plugin/network-viewer-topology.h
        src/collectors/network-viewer.plugin/network-viewer.c
)

# network-viewer: Linux
set(NETWORK_VIEWER_LINUX_FILES
        src/libnetdata/local-sockets/local-sockets.h
        src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.c
        src/collectors/collectors-ipc/ebpfgo_shared_memory.c
        src/collectors/collectors-ipc/ebpfgo_shared_memory.h
        src/collectors/collectors-ipc/ebpfgo_dns_shared_memory.c
        src/collectors/collectors-ipc/ebpfgo_dns_shared_memory.h
        src/collectors/network-viewer.plugin/network_viewer_ebpf_shared_memory.c
        src/collectors/network-viewer.plugin/network_viewer_ebpf_shared_memory.h
        src/collectors/network-viewer.plugin/network_viewer_dns_shared_memory.c
        src/collectors/network-viewer.plugin/network_viewer_dns_shared_memory.h
)

# network-viewer: Windows. This list is self-contained on purpose - the
# OS_WINDOWS branch is the one that does not append NETWORK_VIEWER_COMMON_FILES.
set(NETWORK_VIEWER_WINDOWS_FILES
        src/collectors/network-viewer.plugin/network-viewer-windows.c
        src/collectors/common-cgroups/cgroup-path.c
        src/collectors/common-cgroups/cgroup-path.h
        src/collectors/common-cgroups/cgroup-topology-rules.c
        src/collectors/common-cgroups/cgroup-topology-rules.h
        src/collectors/network-viewer.plugin/network-viewer-apps-lookup-client.h
        src/collectors/network-viewer.plugin/network-viewer-topology-containers.c
        src/collectors/network-viewer.plugin/network-viewer-topology-containers.h
        src/collectors/network-viewer.plugin/network-viewer-topology.c
        src/collectors/network-viewer.plugin/network-viewer-topology.h
        src/libnetdata/local-sockets/local-sockets-windows.h
)

# network-viewer: FreeBSD
set(NETWORK_VIEWER_FREEBSD_FILES
        src/libnetdata/local-sockets/local-sockets-freebsd.h
)

# network-viewer: macOS
set(NETWORK_VIEWER_MACOS_FILES
        src/libnetdata/local-sockets/local-sockets-macos.h
)

if(ENABLE_PLUGIN_NETWORK_VIEWER)
        if(USE_LTO AND CMAKE_C_COMPILER_ID STREQUAL "GNU" AND CMAKE_SIZEOF_VOID_P EQUAL 4)
                # GCC still performs LTO when linking slim LTO static archives into a
                # non-LTO target. Build these archives as fat objects and force a
                # regular final link to avoid exhausting the i386 linker.
                foreach(_network_viewer_fat_lto_target judy libnetdata netipc json-c)
                        if(TARGET ${_network_viewer_fat_lto_target})
                                target_compile_options(${_network_viewer_fat_lto_target} PRIVATE -ffat-lto-objects)
                        endif()
                endforeach()
        endif()

        set(NETWORK_VIEWER_FILES)
        if(OS_LINUX)
                list(APPEND NETWORK_VIEWER_FILES
                        ${NETWORK_VIEWER_LINUX_FILES}
                        ${NETWORK_VIEWER_COMMON_FILES}
                )
        endif()
        if(OS_WINDOWS)
                list(APPEND NETWORK_VIEWER_FILES
                        ${NETWORK_VIEWER_WINDOWS_FILES}
                )
        endif()
        if(OS_FREEBSD)
                list(APPEND NETWORK_VIEWER_FILES
                        ${NETWORK_VIEWER_FREEBSD_FILES}
                        ${NETWORK_VIEWER_COMMON_FILES}
                )
        endif()
        if(OS_MACOS)
                list(APPEND NETWORK_VIEWER_FILES
                        ${NETWORK_VIEWER_MACOS_FILES}
                        ${NETWORK_VIEWER_COMMON_FILES}
                )
        endif()

        add_executable(network-viewer.plugin ${NETWORK_VIEWER_FILES})
        if(USE_LTO AND CMAKE_C_COMPILER_ID STREQUAL "GNU" AND CMAKE_SIZEOF_VOID_P EQUAL 4)
                set_property(TARGET network-viewer.plugin PROPERTY INTERPROCEDURAL_OPTIMIZATION FALSE)
                target_link_options(network-viewer.plugin PRIVATE -fno-lto)
        endif()

        target_compile_options(network-viewer.plugin PRIVATE
                               "$<$<BOOL:${MNL_FOUND}>:${MNL_CFLAGS_OTHER}>")
        target_include_directories(network-viewer.plugin PRIVATE
                                   "$<$<BOOL:${MNL_FOUND}>:${MNL_INCLUDE_DIRS}>")
        target_link_libraries(network-viewer.plugin libnetdata
                              "$<$<BOOL:${MNL_FOUND}>:${MNL_LIBRARIES}>")

        install(TARGETS network-viewer.plugin
                COMPONENT plugin-network-viewer
                DESTINATION ${PLUGINS_DEST})

        if(OS_LINUX)
                add_executable(network-viewer-apps-lookup-client-test EXCLUDE_FROM_ALL
                        src/collectors/network-viewer.plugin/tests/test_network_viewer_apps_lookup_client.c)
                target_link_libraries(network-viewer-apps-lookup-client-test libnetdata)

                add_executable(network-viewer-topology-containers-test EXCLUDE_FROM_ALL
                        src/collectors/network-viewer.plugin/tests/test_network_viewer_topology_containers.c
                        src/collectors/common-cgroups/cgroup-path.c
                        src/collectors/common-cgroups/cgroup-topology-rules.c
                        src/collectors/network-viewer.plugin/network-viewer-topology-containers.c)
                target_link_libraries(network-viewer-topology-containers-test libnetdata)
        endif()

        netdata_add_deb_copyright(plugin-network-viewer netdata-plugin-network-viewer)
endif()
