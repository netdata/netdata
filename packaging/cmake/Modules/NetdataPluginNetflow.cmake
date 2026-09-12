# SPDX-License-Identifier: GPL-3.0-or-later
# netflow-plugin: the Rust flow-analysis plugin, its config, and the Go topology-ip-intel downloader that ships beside it.
#
# include()d from the root file, so paths resolve against the repository and
# build roots; nothing here may use CMAKE_CURRENT_LIST_DIR.

include_guard()

if(ENABLE_PLUGIN_NETFLOW)
    corrosion_install(TARGETS netflow-plugin
                      PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE
                      RUNTIME DESTINATION ${PLUGINS_DEST}
                      COMPONENT plugin-netflow)

    install(FILES src/crates/netflow-plugin/configs/netflow.yaml
            COMPONENT plugin-netflow
            DESTINATION ${LIBCONFIG_DEST})

    if (OS_WINDOWS)
        set(TOPOLOGY_IP_INTEL_DOWNLOADER_BIN topology-ip-intel-downloader.exe)
    else()
        set(TOPOLOGY_IP_INTEL_DOWNLOADER_BIN topology-ip-intel-downloader)
    endif()

    if(CMAKE_SIZEOF_VOID_P LESS 8)
        message(STATUS
                "plugin-netflow: skipping topology-ip-intel-downloader binary on 32-bit targets; "
                "stock topology-ip-intel payload installation remains enabled")
    else()
        add_go_target(topology-ip-intel-downloader-target ${TOPOLOGY_IP_INTEL_DOWNLOADER_BIN} src/go tools/topology-ip-intel-downloader)
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/${TOPOLOGY_IP_INTEL_DOWNLOADER_BIN}
                COMPONENT plugin-netflow
                DESTINATION "${BINDIR}")
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/${PLUGINS_DEST}/topology-ip-intel-downloader\")"
                COMPONENT plugin-netflow)
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/${PLUGINS_DEST}/topology-ip-intel-downloader.plugin\")"
                COMPONENT plugin-netflow)
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/${PLUGINS_DEST}/topology-ip-intel-downloader.exe\")"
                COMPONENT plugin-netflow)
        install(CODE "file(REMOVE \"\$ENV{DESTDIR}\${CMAKE_INSTALL_PREFIX}/${PLUGINS_DEST}/topology-ip-intel-downloader.plugin.exe\")"
                COMPONENT plugin-netflow)
    endif()

    install(FILES src/go/tools/topology-ip-intel-downloader/configs/topology-ip-intel.yaml
            COMPONENT plugin-netflow
            DESTINATION ${LIBCONFIG_DEST})

    if(NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR)
        set(TOPOLOGY_IP_INTEL_STOCK_PAYLOAD
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/README.md"
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/topology-ip-asn.mmdb"
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/topology-ip-geo.mmdb"
                "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}/topology-ip-intel.json")
        foreach(stock_file IN LISTS TOPOLOGY_IP_INTEL_STOCK_PAYLOAD)
            if(NOT EXISTS "${stock_file}")
                message(FATAL_ERROR
                        "Missing topology IP intelligence stock payload file: ${stock_file}")
            endif()
        endforeach()
        install(FILES
                ${TOPOLOGY_IP_INTEL_STOCK_PAYLOAD}
                COMPONENT plugin-netflow
                DESTINATION ${STOCK_DATA_DEST}/topology-ip-intel)
    else()
        message(STATUS
                "plugin-netflow: no staged topology IP intelligence stock payload provided; "
                "skipping stock topology-ip-intel install")
    endif()

    netdata_add_deb_copyright(plugin-netflow netdata-plugin-netflow)
endif()
