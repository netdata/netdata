# SPDX-License-Identifier: GPL-3.0-or-later
# apps: per-process resource monitoring, with its cgroups and eBPF-lookup sources.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together; that is the only form of source-list extraction that does not risk the NETDATA_FILES/LIBNETDATA_FILES composition.

if(ENABLE_PLUGIN_APPS)
    set(APPS_PLUGIN_FILES
            src/collectors/apps.plugin/apps_plugin.c
            src/collectors/apps.plugin/apps_plugin.h
            src/collectors/apps.plugin/apps_functions.c
            src/collectors/apps.plugin/apps_targets.c
            src/collectors/apps.plugin/apps_output.c
            src/collectors/apps.plugin/apps_pid_files.c
            src/collectors/apps.plugin/apps_pid.c
            src/collectors/apps.plugin/apps_aggregations.c
            src/collectors/apps.plugin/apps_os_linux.c
            src/collectors/apps.plugin/apps_os_freebsd.c
            src/collectors/apps.plugin/apps_os_macos.c
            src/collectors/apps.plugin/apps_os_windows.c
            src/collectors/apps.plugin/apps_incremental_collection.c
            src/collectors/apps.plugin/apps_os_windows_nt.c
            src/collectors/apps.plugin/apps_pid_match.c
    )

    if(OS_LINUX)
        list(APPEND APPS_PLUGIN_FILES
                src/collectors/collectors-ipc/ebpfgo_shared_memory.h
                src/collectors/collectors-ipc/ebpfgo_shared_memory.c
                src/collectors/apps.plugin/apps_ebpf_shared_memory.c
                src/collectors/common-cgroups/cgroup-path.c
                src/collectors/common-cgroups/cgroup-path.h
                src/collectors/common-cgroups/cgroup-topology-rules.c
                src/collectors/common-cgroups/cgroup-topology-rules.h
                src/collectors/apps.plugin/apps-cgroups-path.c
                src/collectors/apps.plugin/apps-cgroups-path.h
                src/collectors/apps.plugin/apps-cgroups-enrichment.c
                src/collectors/apps.plugin/apps-cgroups-enrichment.h
                src/collectors/apps.plugin/apps-cgroups-lookup-client.c
                src/collectors/apps.plugin/apps-cgroups-lookup-client.h
                src/collectors/apps.plugin/apps-lookup-netipc.c
                src/collectors/apps.plugin/apps-lookup-netipc.h
        )
    endif()

    add_executable(apps.plugin ${APPS_PLUGIN_FILES})

    target_link_libraries(apps.plugin libnetdata ${CAP_LIBRARIES}
            "$<$<BOOL:${OS_WINDOWS}>:Version;ntdll>")

    target_include_directories(apps.plugin PRIVATE ${CAP_INCLUDE_DIRS})
    target_compile_options(apps.plugin PRIVATE ${CAP_CFLAGS_OTHER})

    install(TARGETS apps.plugin
            COMPONENT plugin-apps
            DESTINATION ${PLUGINS_DEST})

    install(FILES src/collectors/apps.plugin/apps_groups.conf
            COMPONENT plugin-apps
            DESTINATION ${LIBCONFIG_DEST})

    netdata_add_deb_copyright(plugin-apps netdata-plugin-apps)
endif()
