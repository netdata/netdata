#
# CPack options
#

set(CPACK_THREADS 0)

set(CPACK_STRIP_FILES NO)
set(CPACK_DEBIAN_DEBUGINFO_PACKAGE NO)

if(FIELD_DESCR STREQUAL "N/A")
		set(CPACK_PACKAGE_VERSION ${FIELD_MAJOR}.${FIELD_MINOR}.${FIELD_PATCH})
else()
		set(CPACK_PACKAGE_VERSION ${FIELD_MAJOR}.${FIELD_MINOR}.${FIELD_PATCH}-${FIELD_TWEAK}-${FIELD_DESCR})
endif()

set(CPACK_PACKAGING_INSTALL_PREFIX "/")

set(CPACK_PACKAGE_VENDOR "Netdata Inc.")

set(CPACK_RESOURCE_FILE_LICENSE "${CMAKE_SOURCE_DIR}/LICENSE")
set(CPACK_RESOURCE_FILE_README "${CMAKE_SOURCE_DIR}/README.md")

set(CPACK_PACKAGE_INSTALL_DIRECTORY "netdata")
set(CPACK_PACKAGE_DIRECTORY "${CMAKE_BINARY_DIR}/packages")

# to silence lintian
set(CPACK_INSTALL_DEFAULT_DIRECTORY_PERMISSIONS
		OWNER_READ OWNER_WRITE OWNER_EXECUTE
	  GROUP_READ GROUP_EXECUTE
		WORLD_READ WORLD_EXECUTE)

#
# Macro for defining packages
#

function(netdata_declare_package)
    set(options DEBUGINFO NOARCH AUTODEPS INSTALL_CAPS)
    set(oneValueArgs NAME COMPONENT DESCRIPTION SUMMARY)
    set(multiValueArgs DEPENDS RECOMMENDS SUGGESTS CONFLICTS)
    cmake_parse_arguments(DECL_PKG "${options}" "${oneValueArgs}" "${multiValueArgs}" ${ARGN})

    string(TOUPPER "${DECL_PKG_COMPONENT}" _comp)

    set(CPACK_DEBIAN_${_comp}_PACKAGE_SHLIBDEPS ${DECL_PKG_AUTODEPS} PARENT_SCOPE)
    set(CPACK_DEBIAN_${_comp}_PACKAGE_DEBUGINFO ${DECL_PKG_DEBUGINFO} PARENT_SCOPE)

    if(DECL_PKG_NOARCH)
        set(CPACK_DEBIAN_${_comp}_PACKAGE_ARCHITECTURE "all" PARENT_SCOPE)
    endif()

    set(CPACK_DEBIAN_${_comp}_PACKAGE_NAME "${DECL_PKG_NAME}" PARENT_SCOPE)
    set(CPACK_DEBIAN_${_comp}_PACKAGE_DESCRIPTION "${DECL_PKG_SUMMARY}\n${DECL_PKG_DESCRIPTION}" PARENT_SCOPE)

    foreach(dep_type ITEMS DEPENDS RECOMMENDS SUGGESTS CONFLICTS)
        list(JOIN DECL_PKG_${dep_type} ", " dep_list)

        set(CPACK_DEBIAN${_comp}_PACKAGE_${dep_type} "${dep_list}" PARENT_SCOPE)
    endforeach()

    list(APPEND deb_predeps "adduser")

    if(DECL_PKG_INSTALL_CAPS)
        list(APPEND deb_predeps "libcap2-bin")
    endif()

    list(JOIN deb_predeps ", " deb_predeps)
    set(CPACK_DEBIAN_${_comp}_PACKAGE_PREDEPENDS "${deb_predeps}" PARENT_SCOPE)
endfunction()

#
# Debian options
#

set(CPACK_DEB_COMPONENT_INSTALL YES)
set(CPACK_DEBIAN_ENABLE_COMPONENT_DEPENDS YES)
set(CPACK_DEBIAN_FILE_NAME DEB-DEFAULT)

set(CPACK_DEBIAN_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PACKAGE_MAINTAINER "Netdata Builder <bot@netdata.cloud>")

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
set(CPACK_DEBIAN_NETDATA_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_NETDATA_PACKAGE_PREDEPENDS
		"adduser, dpkg (>= 1.17.14), libcap2-bin (>=1:2.0), lsb-base (>= 3.1-23.2)")
set(CPACK_DEBIAN_NETDATA_PACKAGE_SUGGESTS
		"netdata-plugin-cups (= ${CPACK_PACKAGE_VERSION}), netdata-plugin-freeipmi (= ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_NETDATA_PACKAGE_RECOMMENDS
		"netdata-plugin-systemd-journal (= ${CPACK_PACKAGE_VERSION}), \
netdata-plugin-logs-management (= ${CPACK_PACKAGE_VERSION}), \
netdata-plugin-network-viewer (= ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_NETDATA_PACKAGE_CONFLICTS
		"netdata-core, netdata-plugins-bash, netdata-plugins-python, netdata-web")

list(APPEND _main_deps "netdata-plugin-chartsd (= ${CPACK_PACKAGE_VERSION})")
list(APPEND _main_deps "netdata-plugin-pythond (= ${CPACK_PACKAGE_VERSION})")

if(ENABLE_PLUGIN_APPS)
        list(APPEND _main_deps "netdata-plugin-apps (= ${CPACK_PACKAGE_VERSION})")
endif()

if(ENABLE_PLUGIN_GO)
        list(APPEND _main_deps "netdata-plugin-go (= ${CPACK_PACKAGE_VERSION})")
endif()

if(ENABLE_PLUGIN_DEBUGFS)
        list(APPEND _main_deps "netdata-plugin-debugfs (= ${CPACK_PACKAGE_VERSION})")
endif()

if(ENABLE_PLUGIN_NFACCT)
        list(APPEND _main_deps "netdata-plugin-nfacct (= ${CPACK_PACKAGE_VERSION})")
endif()

if(ENABLE_PLUGIN_SLABINFO)
        list(APPEND _main_deps "netdata-plugin-slabinfo (= ${CPACK_PACKAGE_VERSION})")
endif()

if(ENABLE_PLUGIN_PERF)
        list(APPEND _main_deps "netdata-plugin-perf (= ${CPACK_PACKAGE_VERSION})")
endif()

if(ENABLE_PLUGIN_EBPF)
        list(APPEND _main_deps "netdata-plugin-ebpf (= ${CPACK_PACKAGE_VERSION})")
endif()

list(JOIN _main_deps ", " CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS)

set(CPACK_DEBIAN_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/conffiles;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/preinst"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/postinst"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/netdata/postrm")

set(CPACK_DEBIAN_NETDATA_DEBUGINFO_PACKAGE On)

#
# apps.plugin
#

set(CPACK_COMPONENT_PLUGIN-APPS_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-apps
    NAME netdata-plugin-apps
    SUMMARY "The per-application metrics collector plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect per-application and
 per-user metrics without using cgroups."
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/apps/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/apps/postinst")

#
# charts.d.plugin
#

set(CPACK_COMPONENT_PLUGIN-CHARTSD_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-chartsd
    NAME netdata-plugin-chartsd
    SUMMARY "The charts.d metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" This plugin adds a selection of additional collectors written in shell
 script to the Netdata Agent. It includes collectors for LibreSWAN,
 OpenSIPS, and Wireless access point statistics."
    NOARCH
    RECOMMENDS bash
    SUGGESTS iw sudo
)

set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_CONFLICTS "netdata (<< 1.40")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/charts.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/charts.d/postinst")

#
# cups.plugin
#

set(CPACK_COMPONENT_PLUGIN-CUPS_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-cups
    NAME netdata-plugin-cups
    SUMMARY "The CUPS metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect metrics from the Common UNIX Printing System."
    AUTODEPS
    DEBUGINFO
)

set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/cups/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/cups/postinst")

#
# debugfs.plugin
#

set(CPACK_COMPONENT_PLUGIN-DEBUGFS_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-debugfs
    NAME netdata-plugin-debugfs
    SUMMARY "The debugfs metrics collector for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect Linux kernel metrics
 exposed through debugfs."
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/debugfs/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/debugfs/postinst")

#
# ebpf.plugin
#

set(CPACK_COMPONENT_PLUGIN-EBPF_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-ebpf
    NAME netdata-plugin-ebpf
    SUMMARY "The eBPF metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to use eBPF code to collect more
 detailed kernel-level metrics for the system."
    RECCOMMENDS netdata-plugin-apps netdata-ebpf-code-legacy
    AUTODEPS
    DEBUGINFO
)

set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf.d/postinst")

#
# ebpf-code-legacy
#

set(CPACK_COMPONENT_EBPF-CODE-LEGACY_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT ebpf-code-legacy
    NAME netdata-ebpf-code-legacy
    SUMMARY "Compiled eBPF legacy code for the Netdata eBPF plugin"
    DESCRIPTION
" This package provides the pre-compiled eBPF legacy code for use by
 the Netdata eBPF plugin.  This code is only needed when using the eBPF
 plugin with kernel that do not include BTF support (mostly kernel
 versions lower than 5.10)."
    RECOMMENDS netdata-plugin-ebpf
)

set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf-code-legacy/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/ebpf-code-legacy/postinst")

#
# freeipmi.plugin
#

set(CPACK_COMPONENT_PLUGIN-FREEIPMI_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-freeipmi
    NAME netdata-plugin-freeipmi
    SUMMARY "The FreeIPMI metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect metrics from hardware
 using FreeIPMI."
    DEBUGINFO
    AUTODEPS
)

set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/freeipmi/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/freeipmi/postinst")

#
# go.plugin
#

set(CPACK_COMPONENT_PLUGIN-GO_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-go
    NAME netdata-plugin-go
    SUMMARY "The go.d metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" This plugin adds a selection of additional collectors written in Go to
 the Netdata Agent. A significant percentage of the application specific
 collectors provided by Netdata are part of this plugin, so most users
 will want it installed."
    SUGGESTS nvme-cli sudo
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/go.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/go.d/postinst")

#
# logs-management.plugin
#

set(CPACK_COMPONENT_PLUGIN-LOGS-MANAGEMENT_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-logs-management
    NAME netdata-plugin-logs-management
    SUMMARY "The logs-management plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect logs from the system
 and parse them to extract metrics."
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-LOGS-MANAGEMENT_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/logs-management/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/logs-management/postinst")

#
# network-viewer.plugin
#

set(CPACK_COMPONENT_PLUGIN-NETWORK-VIEWER_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-network-viewer
    NAME netdata-plugin-network-viewer
    SUMMARY "The network viewer plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to provide network connection
 mapping functionality for use in Netdata Cloud."
    RECOMMENDS netdata-plugin-ebpf
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/network-viewer/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/network-viewer/postinst")

#
# nfacct.plugin
#

set(CPACK_COMPONENT_PLUGIN-NFACCT_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-nfacct
    NAME netdata-plugin-nfacct
    SUMMARY "The NFACCT metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect metrics from the firewall
 using NFACCT objects."
    DEBUGINFO
    AUTODEPS
)

set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/nfacct/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/nfacct/postinst")

#
# perf.plugin
#

set(CPACK_COMPONENT_PLUGIN-PERF_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-perf
    NAME netdata-plugin-perf
    SUMMARY "The perf metrics collector for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata to collect metrics from the Linux perf
 subsystem."
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/perf/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/perf/postinst")

#
# pythond.plugin
#

set(CPACK_COMPONENT_PLUGIN-PYTHOND_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-pythond
    NAME netdata-plugin-pythond
    SUMMARY "The python.d metrics collection plugin for the Netdata Agent"
    DESCRIPTION
" Many of the collectors provided by this package are also available
 in netdata-plugin-god. In msot cases, you probably want to use those
 versions instead of the Python versions."
    DEPENDS python3
    SUGGESTS sudo
    NOARCH
)

set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/python.d/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/python.d/postinst")

#
# slabinfo.plugin
#

set(CPACK_COMPONENT_PLUGIN-SLABINFO_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-slabinfo
    NAME netdata-plugin-slabinfo
    SUMMARY "The slabinfo metrics collector for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect perfromance and
 utilization metrics for the Linux kernel’s SLAB allocator."
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/slabinfo/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/slabinfo/postinst")

#
# systemd-journal.plugin
#

set(CPACK_COMPONENT_PLUGIN-SYSTEMD-JOURNAL_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-systemd-journal
    NAME netdata-plugin-systemd-journal
    SUMMARY "The systemd-journal collector for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to present logs from the systemd
 journal on Netdata Cloud or the local Agent dashboard."
    DEBUGINFO
    AUTODEPS
    INSTALL_CAPS
)

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/systemd-journal/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/systemd-journal/postinst")

#
# xenstat.plugin
#

set(CPACK_COMPONENT_PLUGIN-XENSTAT_DEPENDS "netdata")

netdata_declare_package(
    COMPONENT plugin-xenstat
    NAME netdata-plugin-xenstat
    SUMMARY "The xenstat plugin for the Netdata Agent"
    DESCRIPTION
" This plugin allows the Netdata Agent to collect metrics from the Xen
 Hypervisor."
    DEBUGINFO
    AUTODEPS
)

set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_CONTROL_EXTRA
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/xenstat/preinst;"
	  "${CMAKE_SOURCE_DIR}/packaging/cmake/control/xenstat/postinst")

#
# CPack components
#

list(APPEND CPACK_COMPONENTS_ALL "netdata")
if(ENABLE_PLUGIN_APPS)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-apps")
endif()
list(APPEND CPACK_COMPONENTS_ALL "plugin-chartsd")
if(ENABLE_PLUGIN_CUPS)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-cups")
endif()
if(ENABLE_PLUGIN_DEBUGFS)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-debugfs")
endif()
if(ENABLE_PLUGIN_EBPF)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-ebpf")
endif()
if(ENABLE_EBPF_LEGACY_PROGRAMS)
        list(APPEND CPACK_COMPONENTS_ALL "ebpf-code-legacy")
endif()
if(ENABLE_PLUGIN_FREEIPMI)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-freeipmi")
endif()
if(ENABLE_PLUGIN_GO)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-go")
endif()
if(ENABLE_PLUGIN_LOGS_MANAGEMENT)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-logs-management")
endif()
if(ENABLE_PLUGIN_NETWORK_VIEWER)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-network-viewer")
endif()
if(ENABLE_PLUGIN_NFACCT)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-nfacct")
endif()
if(ENABLE_PLUGIN_PERF)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-perf")
endif()
list(APPEND CPACK_COMPONENTS_ALL "plugin-pythond")
if(ENABLE_PLUGIN_SLABINFO)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-slabinfo")
endif()
if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-systemd-journal")
endif()
if(ENABLE_PLUGIN_XENSTAT)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-xenstat")
endif()

include(CPack)
