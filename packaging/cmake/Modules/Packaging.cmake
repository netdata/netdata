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
# TODO: DEBHELPER
#

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
# apps.plugin
#

# TODO: changelog/copyright
set(CPACK_COMPONENT_APPS_PLUGIN_PLUGIN_DESCRIPTION
		"Description: The per-application metrics collector plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect per-application and
per-user metrics without using cgroups.")

set(CPACK_COMPONENT_APPS_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_APPS_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_APPS_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_APPS_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_APPS_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/apps/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/apps/postinst")

#
# network-viewer.plugin
#

# TODO: - changelog/copyright
#       - recommends netdata-plugin-ebpf
set(CPACK_COMPONENT_NETWORK_VIEWER_PLUGIN_DESCRIPTION
		"Description: The network viewer plugin for the Netdata Agent
This plugin allows the Netdata Agent to provide network connection
mapping functionality for use in Netdata Cloud.")

set(CPACK_COMPONENT_NETWORK_VIEWER_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_NETWORK_VIEWER_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_NETWORK_VIEWER_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_NETWORK_VIEWER_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_NETWORK_VIEWER_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/network-viewer/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/network-viewer/postinst")

#
# nfacct.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_NFACCT_PLUGIN_DESCRIPTION
		"Description: The NFACCT metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from the firewall
using NFACCT objects.")

set(CPACK_COMPONENT_NFACCT_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_NFACCT_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_NFACCT_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_NFACCT_PLUGIN_PACKAGE_PREDEPENDS "adduser")

set(CPACK_DEBIAN_NFACCT_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/nfacct/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/nfacct/postinst")

#
# freeipmi.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_FREEIPMI_PLUGIN_DESCRIPTION
		"Description: The FreeIPMI metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect metrics from hardware
using FreeIPMI.")

set(CPACK_COMPONENT_FREEIPMI_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_FREEIPMI_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_FREEIPMI_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_FREEIPMI_PLUGIN_PACKAGE_PREDEPENDS "adduser")

set(CPACK_DEBIAN_FREEIPMI_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/freeipmi/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/freeipmi/postinst")

#
# logs-management.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_LOGS_MANAGEMENT_PLUGIN_DESCRIPTION
		"Description: The logs-management plugin for the Netdata Agent
This plugin allows the Netdata Agent to collect logs from the system
and parse them to extract metrics.")

set(CPACK_COMPONENT_LOGS_MANAGEMENT_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_LOGS_MANAGEMENT_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_LOGS_MANAGEMENT_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_LOGS_MANAGEMENT_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_LOGS_MANAGEMENT_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/logs-management/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/logs-management/postinst")

#
# perf.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_PERF_PLUGIN_DESCRIPTION
		"Description: The perf metrics collector for the Netdata Agent
This plugin allows the Netdata to collect metrics from the Linux perf
subsystem.")

set(CPACK_COMPONENT_PERF_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_PERF_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PERF_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PERF_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PERF_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/perf/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/perf/postinst")

#
# systemd-journal.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_SYSTEMD_JOURNAL_PLUGIN_DESCRIPTION
		"Description: The systemd-journal collector for the Netdata Agent
This plugin allows the Netdata Agent to present logs from the systemd
journal on Netdata Cloud or the local Agent dashboard.")

set(CPACK_COMPONENT_SYSTEMD_JOURNAL_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_SYSTEMD_JOURNAL_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_SYSTEMD_JOURNAL_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_SYSTEMD_JOURNAL_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_SYSTEMD_JOURNAL_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/systemd-journal/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/systemd-journal/postinst")

#
# go.d.plugin
#

# TODO: - changelog/copyright
set(CPACK_COMPONENT_GO_D_PLUGIN_DESCRIPTION
		"Description: The go.d metrics collection plugin for the Netdata Agent
This plugin adds a selection of additional collectors written in Go to
the Netdata Agent. A significant percentage of the application specific
collectors provided by Netdata are part of this plugin, so most users
will want it installed.")

set(CPACK_COMPONENT_GO_D_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_GO_D_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_GO_D_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_GO_D_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")
set(CPACK_DEBIAN_GO_D_PLUGIN_PACKAGE_SUGGESTS "nvme-cli, sudo")

set(CPACK_DEBIAN_GO_D_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/go.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/go.d/postinst")

#
# ebpf.plugin
#

# TODO: - changelog/copyright
#       - depend on legacy
set(CPACK_COMPONENT_EBPF_PLUGIN_DESCRIPTION
		"Description: The eBPF metrics collection plugin for the Netdata Agent
This plugin allows the Netdata Agent to use eBPF code to collect more
detailed kernel-level metrics for the system.")

set(CPACK_COMPONENT_EBPF_PLUGIN_DEPENDS "netdata")
set(CPACK_DEBIAN_EBPF_PLUGIN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_EBPF_PLUGIN_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_EBPF_PLUGIN_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_EBPF_PLUGIN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf.d/postinst")

#
# CPack components
#

set(CPACK_COMPONENTS_ALL "netdata;ebpf_plugin;go_d_plugin;systemd_journal_plugin;perf_plugin;logs_management_plugin;freeipmi_plugin;nfacct_plugin;debugfs_plugin;cups_plugin;xenstat_plugin;slabinfo_plugin;apps_plugin;network_viewer_plugin")

include(CPack)
