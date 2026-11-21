# SPDX-License-Identifier: GPL-3.0-or-later
# Shared stock data: DEM/RUM reads GEO independently of the NetFlow plugin.

include_guard()

if(ENABLE_PLUGIN_NETFLOW OR ENABLE_PLUGIN_DEM)
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
                COMPONENT ip-intel-data
                DESTINATION ${STOCK_DATA_DEST}/topology-ip-intel)
        netdata_add_deb_copyright(ip-intel-data netdata-ip-intel-data)
    elseif(NETDATA_PACKAGE_KIND MATCHES "^(deb|rpm)$")
        message(FATAL_ERROR
                "Native NetFlow and DEM packages require NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR")
    else()
        message(STATUS
                "No staged topology IP intelligence stock payload provided; "
                "skipping stock topology-ip-intel install")
    endif()
endif()
