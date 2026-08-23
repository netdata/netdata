# SPDX-License-Identifier: GPL-3.0-or-later
# cgroup-network: the cgroup network-namespace resolver.
#
# Relocated verbatim from the root CMakeLists.txt. include()d rather than
# add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and CMAKE_CURRENT_BINARY_DIR
# keep pointing at the repository and build roots, which every relative path
# below depends on. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# Its source list, executable and install rule move together, so nothing here
# takes part in the NETDATA_FILES/LIBNETDATA_FILES composition.

include_guard()

if(ENABLE_PLUGIN_CGROUP_NETWORK)
    set(CGROUP_NETWORK_FILES src/collectors/cgroups.plugin/cgroup-network.c)

    add_executable(cgroup-network ${CGROUP_NETWORK_FILES})
    target_link_libraries(cgroup-network libnetdata)

    install(TARGETS cgroup-network
            COMPONENT netdata
            DESTINATION ${PLUGINS_DEST})
endif()
