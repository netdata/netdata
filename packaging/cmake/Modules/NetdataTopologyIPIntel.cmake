# SPDX-License-Identifier: GPL-3.0-or-later
# Shared stock data: DEM/RUM reads GEO independently of the NetFlow plugin.

include_guard()

if(ENABLE_PLUGIN_NETFLOW OR (STATIC_BUILD AND ENABLE_PLUGIN_DEM))
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
        # Preserve native package ownership. Static installs include all components.
        install(FILES
                ${TOPOLOGY_IP_INTEL_STOCK_PAYLOAD}
                COMPONENT plugin-netflow
                DESTINATION ${STOCK_DATA_DEST}/topology-ip-intel)
    else()
        message(STATUS
                "No staged topology IP intelligence stock payload provided; "
                "skipping stock topology-ip-intel install")
    endif()
endif()
