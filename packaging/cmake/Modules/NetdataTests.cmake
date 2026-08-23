# SPDX-License-Identifier: GPL-3.0-or-later
# Test executables. None is installed or registered with CTest - the project
# defines no add_test() - but they are NOT all orphans: the topology container
# tests workflow (.github/workflows/topology-container-tests.yml) builds twelve
# of these targets by name.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Built on demand: all but cgroup-lookup-test are EXCLUDE_FROM_ALL, and that one is behind ENABLE_CGROUPS_LOOKUP_TEST_CLIENT, off by default.

include_guard()

if(ENABLE_CGROUPS_LOOKUP_TEST_CLIENT)
    add_executable(cgroup-lookup-test src/collectors/cgroups.plugin/cgroup-lookup-test/cgroup-lookup-test.c)
    target_link_libraries(cgroup-lookup-test libnetdata)
endif()

if(OS_LINUX)
    add_executable(cgroup-name-config-test EXCLUDE_FROM_ALL
            src/collectors/cgroups.plugin/tests/test_cgroup_name_config.c
            src/collectors/cgroups.plugin/cgroup-name-config.c)

    add_executable(cgroup-orchestrator-test EXCLUDE_FROM_ALL
            src/collectors/cgroups.plugin/tests/test_cgroup_orchestrator.c
            src/collectors/cgroups.plugin/cgroup-orchestrator.c)
    target_compile_definitions(cgroup-orchestrator-test PRIVATE NETDATA_INTERNAL_CHECKS=1)
    target_link_libraries(cgroup-orchestrator-test libnetdata)

    # The label-parser test links the real src/database/rrdlabels.c against a
    # small set of stubs (test_cgroups_plugin_stubs.c) so it stays self-contained
    # while still validating production parsing behavior. Plain C, no test
    # framework; the target is EXCLUDE_FROM_ALL so it is built only on demand.
    add_executable(cgroups-plugin-labels-test EXCLUDE_FROM_ALL
            src/collectors/cgroups.plugin/tests/test_cgroups_plugin.c
            src/collectors/cgroups.plugin/tests/test_cgroups_plugin_stubs.c
            src/collectors/cgroups.plugin/cgroup-name-labels.c
            src/database/rrdlabels.c
            src/database/rrdlabels-aggregated.c
            src/database/pattern-array.c)
    target_link_libraries(cgroups-plugin-labels-test libnetdata)

    add_executable(apps-cgroups-path-test EXCLUDE_FROM_ALL
            src/collectors/apps.plugin/tests/test_apps_cgroups_path.c
            src/collectors/common-cgroups/cgroup-path.c
            src/collectors/apps.plugin/apps-cgroups-path.c)
    target_link_libraries(apps-cgroups-path-test libnetdata)

    add_executable(apps-cgroups-enrichment-test EXCLUDE_FROM_ALL
            src/collectors/apps.plugin/tests/test_apps_cgroups_enrichment.c
            src/collectors/common-cgroups/cgroup-path.c
            src/collectors/common-cgroups/cgroup-topology-rules.c)
    target_link_libraries(apps-cgroups-enrichment-test libnetdata)

    add_executable(cgroup-topology-rules-test EXCLUDE_FROM_ALL
            src/collectors/common-cgroups/tests/test_cgroup_topology_rules.c
            src/collectors/common-cgroups/cgroup-path.c
            src/collectors/common-cgroups/cgroup-topology-rules.c)
    target_link_libraries(cgroup-topology-rules-test libnetdata)

    add_executable(apps-lookup-protocol-test EXCLUDE_FROM_ALL
            src/collectors/apps.plugin/tests/test_apps_lookup_protocol.c)
    target_link_libraries(apps-lookup-protocol-test libnetdata)

    add_executable(apps-lookup-netipc-lock-test EXCLUDE_FROM_ALL
            src/collectors/apps.plugin/tests/test_apps_lookup_netipc_lock.c)
    target_link_libraries(apps-lookup-netipc-lock-test libnetdata)

    add_executable(apps-cgroups-lookup-client-abort-test EXCLUDE_FROM_ALL
            src/collectors/apps.plugin/tests/test_apps_cgroups_lookup_client_abort.c)
    target_link_libraries(apps-cgroups-lookup-client-abort-test libnetdata)
endif()

add_executable(stream-receiver-hops-test EXCLUDE_FROM_ALL
        src/streaming/tests/stream-handshake-hops-test.c)
target_link_libraries(stream-receiver-hops-test libnetdata)

add_executable(stream-replay-endpoints-test EXCLUDE_FROM_ALL
        src/streaming/tests/stream-replay-endpoints-test.c)
target_link_libraries(stream-replay-endpoints-test libnetdata)

if(OS_LINUX)
        add_executable(http-auth-bearer-test EXCLUDE_FROM_ALL
                src/web/api/tests/http-auth-bearer-test.c)
        target_compile_options(http-auth-bearer-test PRIVATE -ffunction-sections -fdata-sections)
        target_link_options(http-auth-bearer-test PRIVATE -Wl,--no-export-dynamic -Wl,--gc-sections)
        target_link_libraries(http-auth-bearer-test libnetdata)
endif()

if(ENABLE_CGROUPS_LOOKUP_SERVER)
    add_executable(cgroup-lookup-netipc-test EXCLUDE_FROM_ALL
            src/collectors/cgroups.plugin/tests/test_cgroup_lookup_netipc.c
            src/collectors/cgroups.plugin/cgroup-lookup-netipc.c
            src/collectors/cgroups.plugin/cgroup-lookup-resolver.c
            src/collectors/cgroups.plugin/cgroup-snapshot-store.c
            src/collectors/common-cgroups/cgroup-path.c
            src/collectors/cgroups.plugin/cgroup-orchestrator.c)
    target_compile_definitions(cgroup-lookup-netipc-test PRIVATE NETDATA_INTERNAL_CHECKS=1)
    target_link_libraries(cgroup-lookup-netipc-test libnetdata)
endif()
