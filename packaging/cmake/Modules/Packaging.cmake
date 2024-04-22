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

set(CPACK_PACKAGE_INSTALL_DIRECTORY "netdata")
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

set(CPACK_DEBIAN_NETDATA_PACKAGE_NAME "netdata")
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
set(CPACK_COMPONENT_PLUGIN-DEBUGFS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-DEBUGFS_DESCRIPTION
		"The debugfs metrics collector for the Netdata Agent
This plugin allows the Netdata Agent to collect Linux kernel metrics
exposed through debugfs.")

set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_NAME "netdata-plugin-debugfs")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_PREDEPENDS "libcap2-bin, adduser")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/debugfs/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/debugfs/postinst")

#
# cups.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_PLUGIN-CUPS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-CUPS_DESCRIPTION
	  "The CUPS metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from the Common UNIX Printing System.")

set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_NAME "netdata-plugin-cups")
set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_SECTION "net")
# set(CPACK_DEBIAN_CUPS_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_PREDEPENDS "adduser")
set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/cups/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/cups/postinst")

#
# xenstat.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_PLUGIN-XENSTAT_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-XENSTAT_DESCRIPTION
		"The xenstat plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from the Xen
Hypervisor.")

set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_NAME "netdata-plugin-xenstat")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_PREDEPENDS "adduser")

set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/xenstat/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/xenstat/postrm")

#
# slabinfo.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_PLUGIN-SLABINFO_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-SLABINFO_DESCRIPTION
		"Description: The slabinfo metrics collector for the Netdata Agent
This plugin allows the Netdata Agent to collect perfromance and
utilization metrics for the Linux kernel’s SLAB allocator.")

set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_NAME "netdata-plugin-slabinfo")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/slabinfo/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/slabinfo/postinst")

#
# apps.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_PLUGIN-APPS_DESCRIPTION
		"Description: The per-application metrics collector plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect per-application and
per-user metrics without using cgroups.")

set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_NAME "netdata-plugin-apps")
set(CPACK_COMPONENT_PLUGIN-APPS_DEPENDS "netdata")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/apps/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/apps/postinst")

#
# network-viewer.plugin
#

# TODO: - changelog/copyright
#       - recommends netdata-plugin-ebpf
set(CPACK_COMPONENT_PLUGIN-NETWORK-VIEWER_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-NETWORK-VIEWER_DESCRIPTION
		"Description: The network viewer plugin for the Netdata Agent
This plugin allows the Netdata Agent to provide network connection
mapping functionality for use in Netdata Cloud.")

set(CPACK_DEBIAN_PLUGIN-NETWORK_VIEWER_PACKAGE_NAME "netdata-plugin-network-viewer")
set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/network-viewer/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/network-viewer/postinst")

#
# nfacct.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-NFACCT_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-NFACCT_DESCRIPTION
		"Description: The NFACCT metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from the firewall
using NFACCT objects.")

set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_NAME "netdata-plugin-nfacct")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_PREDEPENDS "adduser")

set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/nfacct/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/nfacct/postinst")

#
# freeipmi.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-FREEIPMI_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-FREEIPMI_DESCRIPTION
		"Description: The FreeIPMI metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from hardware
using FreeIPMI.")

set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_NAME "netdata-plugin-freeipmi")
set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_PREDEPENDS "adduser")

set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/freeipmi/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/freeipmi/postinst")

#
# logs-management.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-LOGS-MANAGEMENT_DEPENDS "netdata")
set(CPACK_COMPONENT_LOGS_MANAGEMENT_DESCRIPTION
		"Description: The logs-management plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect logs from the system
and parse them to extract metrics.")

set(CPACK_DEBIAN_PLUGIN-LOGS-MANAGEMENT_PACKAGE_NAME "netdata-plugin-logs-management")
set(CPACK_DEBIAN_PLUGIN-LOGS-MANAGEMENT_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-LOGS-MANAGEMENT_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-LOGS-MANAGEMENT_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-LOGS-MANAGEMENT_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/logs-management/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/logs-management/postinst")

#
# perf.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-PERF_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-PERF_DESCRIPTION
		"Description: The perf metrics collector for the Netdata Agent
This plugin allows the Netdata to collect metrics from the Linux perf
subsystem.")

set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_NAME "netdata-plugin-perf")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/perf/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/perf/postinst")

#
# systemd-journal.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-SYSTEMD-JOURNAL_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-SYSTEMD-JOURNAL_DESCRIPTION
		"Description: The systemd-journal collector for the Netdata Agent
This plugin allows the Netdata Agent to present logs from the systemd
journal on Netdata Cloud or the local Agent dashboard.")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_NAME "netdata-plugin-systemd-journal")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/systemd-journal/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/systemd-journal/postinst")

#
# go.d.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-GO_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-GO_DESCRIPTION
		"Description: The go.d metrics collection plugin for the Netdata Agent
This plugin adds a selection of additional collectors written in Go to
the Netdata Agent. A significant percentage of the application specific
collectors provided by Netdata are part of this plugin, so most users
will want it installed.")

set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_NAME "netdata-plugin-go")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_PREDEPENDS "libcap2-bin, adduser")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_SUGGESTS "nvme-cli, sudo")

set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/go.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/go.d/postinst")

#
# ebpf.plugin
#

# TODO: - changelog/copyright
#       - depend on legacy
set(CPACK_COMPONENT_PLUGIN-EBPF_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-EBPF_DESCRIPTION
		"Description: The eBPF metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to use eBPF code to collect more
detailed kernel-level metrics for the system.")

set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_NAME "netdata-plugin-ebpf")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf.d/postinst")

#
# charts.d.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-CHARTSD_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-CHARTSD_DESCRIPTION
		"Description: The charts.d metrics collection plugin for the Netdata Agent
This plugin adds a selection of additional collectors written in shell
script to the Netdata Agent. It includes collectors for APCUPSD,
LibreSWAN, OpenSIPS, and Wireless access point statistics.")

set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_NAME "netdata-plugin-chartsd")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_PREDEPENDS "adduser")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_DEPENDS "bash")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_ARCHITECTURE "all")

set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/charts.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/charts.d/postinst")

#
# python.d.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PLUGIN-PYTHOND_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-PYTHOND_DESCRIPTION
		"Description: The python.d metrics collection plugin for the Netdata Agent
Many of the collectors provided by this package are also available
in netdata-plugin-god. In msot cases, you probably want to use those
versions instead of the Python versions.")

set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_NAME "netdata-plugin-pythond")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_PREDEPENDS "adduser")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_SUGGESTS "sudo")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_ARCHITECTURE "all")

set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/python.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/python.d/postinst")

#
# CPack components
#

list(APPEND CPACK_COMPONENTS_ALL "netdata")
list(APPEND CPACK_COMPONENTS_ALL "plugin-pythond")
list(APPEND CPACK_COMPONENTS_ALL "plugin-chartsd")
list(APPEND CPACK_COMPONENTS_ALL "plugin-ebpf")
list(APPEND CPACK_COMPONENTS_ALL "plugin-go")
list(APPEND CPACK_COMPONENTS_ALL "plugin-systemd-journal")
list(APPEND CPACK_COMPONENTS_ALL "plugin-perf")
list(APPEND CPACK_COMPONENTS_ALL "plugin-logs-management")
list(APPEND CPACK_COMPONENTS_ALL "plugin-freeipmi")
list(APPEND CPACK_COMPONENTS_ALL "plugin-nfacct")
list(APPEND CPACK_COMPONENTS_ALL "plugin-debugfs")
list(APPEND CPACK_COMPONENTS_ALL "plugin-cups")
list(APPEND CPACK_COMPONENTS_ALL "plugin-xenstat")
list(APPEND CPACK_COMPONENTS_ALL "plugin-slabinfo")
list(APPEND CPACK_COMPONENTS_ALL "plugin-apps")
list(APPEND CPACK_COMPONENTS_ALL "plugin-network-viewer")

include(CPack)
