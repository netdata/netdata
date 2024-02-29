#
# CPack options
#

set(CPACK_STRIP_FILES YES)
set(CPACK_DEBIAN_PACKAGE_SHLIBDEPS YES)

set(CPACK_PACKAGE_VERSION "6.7.8")
set(CPACK_PACKAGE_VERSION_MAJOR "11")
set(CPACK_PACKAGE_VERSION_MINOR "22")
set(CPACK_PACKAGE_VERSION_PATCH "33")

set(CPACK_PACKAGING_INSTALL_PREFIX "/")

set(CPACK_PACKAGE_VENDOR "Netdata Inc.")

set(CPACK_RESOURCE_FILE_LICENSE "${CMAKE_SOURCE_DIR}/LICENSE")
set(CPACK_RESOURCE_FILE_README "${CMAKE_SOURCE_DIR}/README.md")

set(CPACK_PACKAGE_NAME "netdata")

set(CPACK_PACKAGE_INSTALL_DIRECTORY ${CPACK_PACKAGE_NAME})
set(CPACK_PACKAGE_DIRECTORY "${CMAKE_SOURCE_DIR}/artifacts")

# to silence lintian
set(CPACK_INSTALL_DEFAULT_DIRECTORY_PERMISSIONS
		OWNER_READ OWNER_WRITE OWNER_EXECUTE
	  GROUP_READ GROUP_EXECUTE
		WORLD_READ WORLD_EXECUTE)

#
# Debian options
#

set(CPACK_DEB_COMPONENT_INSTALL YES)
set(CPACK_DEBIAN_ENABLE_COMPONENT_DEPENDS YES)
set(CPACK_DEBIAN_FILE_NAME DEB-DEFAULT)

set(CPACK_DEBIAN_PACKAGE_MAINTAINER "Netdata Builder <bot@netdata.cloud>")

#
# dependencies
#

#
# netdata
#

set(CPACK_COMPONENT_NETDATA_DESCRIPTION
	  "real-time charts for system monitoring
Netdata is a daemon that collects data in realtime (per second)
and presents a web site to view and analyze them. The presentation
is also real-time and full of interactive charts that precisely
render all collected values.")

set(CPACK_DEBIAN_NETDATA_PACKAGE_PREDEPENDS
		"adduser, dpkg (>= 1.17.14), libcap2-bin (>=1:2.0), lsb-base (>= 3.1-23.2)")
# set(CPACK_DEBIAN_NETDATA_PACKAGE_SUGGESTS
# 		"netdata-plugin-cups (= ${CPACK_PACKAGE_VERSION}), netdata-plugin-freeipmi (= ${CPACK_PACKAGE_VERSION})")
# set(CPACK_DEBIAN_NETDATA_PACKAGE_RECOMMENDS
# 		"netdata-plugin-systemd-journal (= ${CPACK_PACKAGE_VERSION}), netdata-plugin-logs-management (= ${CPACK_PACKAGE_VERSION})")
# set(CPACK_DEBIAN_NETDATA_PACKAGE_CONFLICTS
# 		"netdata-core, netdata-plugins-bash, netdata-plugins-python, netdata-web")

# set(CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS
# 		"netdata-plugin-apps (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-pythond (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-go (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-debugfs (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-nfacct (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-chartsd (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-slabinfo (= ${CPACK_PACKAGE_VERSION}), \
# netdata-plugin-perf (= ${CPACK_PACKAGE_VERSION})")

# if(CMAKE_SYSTEM_PROCESSOR MATCHES "x86_64|amd64|AMD64")
# 		set(CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS
#     		"netdata-plugin-ebpf (= ${CPACK_PACKAGE_VERSION}), ${CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS}")
# endif()

# set(CPACK_DEBIAN_PACKAGE_CONTROL_EXTRA
# 	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/conffiles;"
# 	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/preinst"
# 	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/postinst"
# 	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/postrm")

#
# debugfs.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_DEBUGFS_PLUGIN_DESCRIPTION
		"The debugfs metrics collector for the Netdata Agent
This plugin allows the Netdata Agent to collect Linux kernel metrics
exposed through debugfs.")
set(CPACK_COMPONENT_DEBUGFS_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_DEBUGFS_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_DEBUGFS_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_DEBUGFS_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

#
# cups.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_CUPS_PLUGIN_DESCRIPTION
	  "The CUPS metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from the Common UNIX Printing System.")
set(CPACK_COMPONENT_CUPS_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_CUPS_PLUGIN_PACKAGE_SECTION "net")
# set(CPACK_DEBIAN_CUPS_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_CUPS_PLUGIN_PACKAGE_PREDEPENDS "adduser")

#
# xenstat.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_XENSTAT_PLUGIN_DESCRIPTION
		"The xenstat plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from the Xen
Hypervisor.")
set(CPACK_COMPONENT_XENSTAT_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_XENSTAT_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_XENSTAT_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_XENSTAT_PLUGIN_PACKAGE_PREDEPENDS "adduser")

set(CPACK_DEBIAN_XENSTAT_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/xenstat/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/postrm")

#
# slabinfo.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_SLABINFO_PLUGIN_DESCRIPTION
		"Description: The slabinfo metrics collector for the Netdata Agent
This plugin allows the Netdata Agent to collect perfromance and
utilization metrics for the Linux kernel’s SLAB allocator.")

set(CPACK_COMPONENT_SLABINFO_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_SLABINFO_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_SLABINFO_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_SLABINFO_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_SLABINFO_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/slabinfo/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/slabinfo/postinst")

#
# CPack components
#

set(CPACK_COMPONENTS_ALL "netdata;debugfs_plugin;cups_plugin;xenstat_plugin;slabinfo_plugin")

include(CPack)
