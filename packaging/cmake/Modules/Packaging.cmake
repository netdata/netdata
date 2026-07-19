#
# CPack options
#

if(NETDATA_VERSION_DESCR STREQUAL "N/A")
        set(CPACK_PACKAGE_VERSION ${NETDATA_VERSION_MAJOR}.${NETDATA_VERSION_MINOR}.${NETDATA_VERSION_PATCH})
else()
        set(CPACK_PACKAGE_VERSION ${NETDATA_VERSION_MAJOR}.${NETDATA_VERSION_MINOR}.${NETDATA_VERSION_PATCH}-${NETDATA_VERSION_TWEAK}-${NETDATA_VERSION_DESCR})
endif()

set(CPACK_THREADS 0)
set(CPACK_COMPONENTS_GROUPING IGNORE)

set(CPACK_STRIP_FILES NO)
set(CPACK_DEBIAN_DEBUGINFO_PACKAGE NO)

set(CPACK_DEBIAN_PACKAGE_SHLIBDEPS YES)
set(CPACK_DEBIAN_COMPRESSION_TYPE "xz")

include(NetdataOSRelease)

if(OS_LINUX)
  if(OS_DISTRO_ID STREQUAL "debian")
    if(OS_VERSION_ID VERSION_GREATER_EQUAL 12)
      set(CPACK_DEBIAN_COMPRESSION_TYPE "zstd")
    endif()
  elseif(OS_DISTRO_ID STREQUAL "ubuntu")
    if(OS_VERSION_ID VERSION_GREATER_EQUAL 21.10)
      set(CPACK_DEBIAN_COMPRESSION_TYPE "zstd")
    endif()
  endif()
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
# Debian options
#

set(CPACK_DEB_COMPONENT_INSTALL YES)
set(CPACK_DEBIAN_ENABLE_COMPONENT_DEPENDS YES)
set(CPACK_DEBIAN_FILE_NAME DEB-DEFAULT)

set(CPACK_DEBIAN_PACKAGE_MAINTAINER "Netdata Builder <bot@netdata.cloud>")

#
# RPM options
#
# The RPM configuration mirrors netdata.spec.in package for package; the spec
# stays authoritative for the rpmbuild (v1 builder) path until CI flips to
# this one.
#

set(CPACK_RPM_COMPONENT_INSTALL YES)
set(CPACK_RPM_FILE_NAME RPM-DEFAULT)

# RPM forbids dashes in Version; the spec receives a pre-sanitized version
# (nightly 2.10.0-809-nightly becomes 2.10.0.809.nightly).
string(REPLACE "-" "." CPACK_RPM_PACKAGE_VERSION "${CPACK_PACKAGE_VERSION}")
set(CPACK_RPM_PACKAGE_RELEASE 1)
set(CPACK_RPM_PACKAGE_RELEASE_DIST ON)

set(CPACK_RPM_PACKAGE_LICENSE "GPLv3+")
set(CPACK_RPM_PACKAGE_GROUP "Applications/System")
set(CPACK_RPM_PACKAGE_URL "http://my-netdata.io")
set(CPACK_RPM_CHANGELOG_FILE "${PKG_FILES_PATH}/rpm/changelog")

# The spec disables all of __os_install_post (no stripping, no debuginfo
# extraction, no brp scripts) and tolerates Go binaries without RPM build ids.
set(CPACK_RPM_SPEC_MORE_DEFINE "%global __os_install_post %{nil}
%global _missing_build_ids_terminate_build 0")

# /usr/lib/netdata/system ships reference service files for many init systems;
# the spec excludes them from automatic dependency scanning so their
# interpreters do not become package requirements.
set(CPACK_RPM_REQUIRES_EXCLUDE_FROM "^/usr/lib/netdata/system/.*$")

# Shared system directories the packages must not claim ownership of, on top
# of CPack's builtin list (/etc, /usr, /usr/lib, /usr/share, ...). The staged
# custom-plugins.d and ssl dirs are unpackaged in the spec build and are
# excluded for parity with it.
set(CPACK_RPM_EXCLUDE_FROM_AUTO_FILELIST_ADDITION
    # an install-rule-less component (plugin-journal-viewer) degenerates to a
    # lone "/" entry
    /
    /etc/netdata
    /etc/netdata/custom-plugins.d
    /etc/netdata/ssl
    /etc/logrotate.d
    /usr/libexec
    /usr/sbin
    /usr/lib/systemd
    /usr/lib/systemd/system
    /usr/lib/systemd/system-preset
    /usr/lib/systemd/journald@netdata.conf.d
    /usr/lib/sysusers.d
    /usr/lib/tmpfiles.d
    /usr/share/netdata
    /var
    /var/cache
    /var/lib
    /var/log
    # staged but unpackaged in the spec build
    /var/run
    /var/run/netdata
    /etc/netdata/charts.d
    /etc/netdata/go.d
    /etc/netdata/python.d
    /etc/netdata/otel.d
    /etc/netdata/otel.d/v1
    /etc/netdata/otel.d/v1/metrics
    # the spec leaves the legacy eBPF object directory unowned
    /usr/libexec/netdata/plugins.d/ebpf.d
    # owned by exactly one package (other components stage them too); the
    # owning package's USER_FILELIST re-adds them as %dir entries
    /usr/libexec/netdata
    /usr/libexec/netdata/plugins.d
    /usr/lib/netdata
    /usr/lib/netdata/conf.d
    /usr/share/netdata/web)

# The spec does not own the vendored IBM MQ directory tree either; the list
# of its directories is derived from the MQ manifest by install_ibm_runtime.
if(NETDATA_IBM_MQ_RPM_DIR_EXCLUDES)
  list(APPEND CPACK_RPM_EXCLUDE_FROM_AUTO_FILELIST_ADDITION
       ${NETDATA_IBM_MQ_RPM_DIR_EXCLUDES})
endif()

# Scriptlets. openSUSE uses the %service_* macro family, everything else the
# %systemd_* one; the choice happens at spec-generation time because the
# macros expand when rpmbuild parses the generated spec. On distros where RPM
# handles the shipped sysusers file natively (EL >= 10, Fedora >= 43) the
# netdata-user %post only manages supplemental groups; elsewhere it also
# creates the user and group, exactly like the spec.
if(NETDATA_DISTRO_SUSE)
  set(NETDATA_RPM_SCRIPTLET_FAMILY "suse")
else()
  set(NETDATA_RPM_SCRIPTLET_FAMILY "systemd")
endif()

if(NOT NETDATA_DISTRO_SUSE AND
   ((NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR GREATER_EQUAL 10) OR
    (NETDATA_DISTRO_FEDORA AND NETDATA_DISTRO_VERSION_MAJOR GREATER_EQUAL 43)))
  set(NETDATA_RPM_USER_POST "post.groups-only")
else()
  set(NETDATA_RPM_USER_POST "post.create-user")
endif()

set(CPACK_RPM_NETDATA_PRE_INSTALL_SCRIPT_FILE
    "${PKG_FILES_PATH}/rpm/netdata/pre")
set(CPACK_RPM_NETDATA_POST_INSTALL_SCRIPT_FILE
    "${PKG_FILES_PATH}/rpm/netdata/post.${NETDATA_RPM_SCRIPTLET_FAMILY}")
set(CPACK_RPM_NETDATA_PRE_UNINSTALL_SCRIPT_FILE
    "${PKG_FILES_PATH}/rpm/netdata/preun.${NETDATA_RPM_SCRIPTLET_FAMILY}")
set(CPACK_RPM_NETDATA_POST_UNINSTALL_SCRIPT_FILE
    "${PKG_FILES_PATH}/rpm/netdata/postun.${NETDATA_RPM_SCRIPTLET_FAMILY}")
set(CPACK_RPM_USER_POST_INSTALL_SCRIPT_FILE
    "${PKG_FILES_PATH}/rpm/user/${NETDATA_RPM_USER_POST}")

# Dependency predicates mirroring the spec's user handling: on sysusers
# platforms the account comes from the shipped sysusers file, elsewhere every
# payload package needs netdata-user installed first.
if(NETDATA_DISTRO_SUSE OR
   (NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR GREATER_EQUAL 10) OR
   (NETDATA_DISTRO_FEDORA AND NETDATA_DISTRO_VERSION_MAJOR GREATER_EQUAL 43))
  set(NETDATA_RPM_HAVE_SYSUSER TRUE)
else()
  set(NETDATA_RPM_HAVE_SYSUSER FALSE)
endif()

# rpm on EL 7 and Amazon Linux 2 has no weak dependencies; the spec turns the
# load-bearing Recommends into hard Requires there and drops the Suggests.
if((NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR LESS_EQUAL 7) OR
   (NETDATA_DISTRO_AMZN AND NETDATA_DISTRO_VERSION_MAJOR LESS_EQUAL 2))
  set(NETDATA_RPM_HAVE_WEAK_DEPS FALSE)
else()
  set(NETDATA_RPM_HAVE_WEAK_DEPS TRUE)
endif()

set(NETDATA_RPM_VERSION "${CPACK_RPM_PACKAGE_VERSION}")
if(NOT NETDATA_RPM_HAVE_SYSUSER)
  set(NETDATA_RPM_USER_PREDEP "netdata-user >= ${NETDATA_RPM_VERSION}")
else()
  set(NETDATA_RPM_USER_PREDEP "")
endif()

#
# netdata
#

set(CPACK_COMPONENT_NETDATA_DESCRIPTION
	  "Netdata is a high-resolution, real-time, low-latency observability platform.
	  Per-second data collection, high-performance long-term storage, low-latency
	  visualization, machine-learning based anomaly detection, alerts and notifications,
	  advanced correlations and fast root cause analysis, native horizontal scalability.")

set(CPACK_DEBIAN_NETDATA_PACKAGE_NAME "netdata")
set(CPACK_DEBIAN_NETDATA_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_NETDATA_PACKAGE_PREDEPENDS "netdata-user, libcap2-bin")
set(CPACK_DEBIAN_NETDATA_PACKAGE_SUGGESTS
		"netdata-plugin-cups, netdata-plugin-freeipmi, netdata-plugin-ibm")
set(CPACK_DEBIAN_NETDATA_PACKAGE_RECOMMENDS
		"netdata-plugin-systemd-journal, netdata-plugin-systemd-units, \
netdata-plugin-network-viewer, netdata-plugin-scripts")
set(CPACK_DEBIAN_NETDATA_PACKAGE_CONFLICTS
		"netdata-core, netdata-plugins-bash, netdata-plugins-python, netdata-web")

if(ENABLE_DASHBOARD)
  list(APPEND _main_deps "netdata-dashboard")
endif()

if(ENABLE_PLUGIN_OTEL)
  list(APPEND _main_deps "netdata-plugin-otel")
endif()

if(ENABLE_PLUGIN_CHARTS)
  list(APPEND _main_deps "netdata-plugin-chartsd")
endif()

if(ENABLE_PLUGIN_PYTHON)
  list(APPEND _main_deps "netdata-plugin-pythond")
endif()

if(ENABLE_PLUGIN_APPS)
        list(APPEND _main_deps "netdata-plugin-apps")
endif()

if(ENABLE_PLUGIN_GO)
        list(APPEND _main_deps "netdata-plugin-go")
endif()

if(ENABLE_PLUGIN_DEBUGFS)
        list(APPEND _main_deps "netdata-plugin-debugfs")
endif()

if(ENABLE_PLUGIN_NFACCT)
        list(APPEND _main_deps "netdata-plugin-nfacct")
endif()

if(ENABLE_PLUGIN_SLABINFO)
        list(APPEND _main_deps "netdata-plugin-slabinfo")
endif()

if(ENABLE_PLUGIN_PERF)
        list(APPEND _main_deps "netdata-plugin-perf")
endif()

if(ENABLE_PLUGIN_EBPF)
        list(APPEND _main_deps "netdata-plugin-ebpf")
endif()

list(JOIN _main_deps ", " CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS)

set(CPACK_DEBIAN_NETDATA_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/netdata/conffiles;"
	  "${PKG_FILES_PATH}/deb/netdata/postinst"
	  "${PKG_FILES_PATH}/deb/netdata/prerm"
	  "${PKG_FILES_PATH}/deb/netdata/postrm")

set(CPACK_DEBIAN_NETDATA_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_NETDATA_PACKAGE_NAME "netdata")
set(CPACK_RPM_NETDATA_PACKAGE_SUMMARY "Netdata - X-Ray Vision for your infrastructure!")
set(CPACK_RPM_NETDATA_PACKAGE_OBSOLETES "netdata-conf, netdata-data, netdata-plugin-logs-management")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_NETDATA_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()

# Mirrors the spec's main-package dependency block, quirks included: the
# netdata-dashboard dependency is unversioned there.
unset(_rpm_main_requires)
unset(_rpm_main_recommends)
unset(_rpm_main_suggests)

if(ENABLE_DASHBOARD)
  list(APPEND _rpm_main_requires "netdata-dashboard")
endif()
if(ENABLE_PLUGIN_EBPF)
  list(APPEND _rpm_main_requires "netdata-plugin-ebpf = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_OTEL)
  list(APPEND _rpm_main_requires "netdata-plugin-otel = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_APPS)
  list(APPEND _rpm_main_requires "netdata-plugin-apps = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_PYTHON)
  list(APPEND _rpm_main_requires "netdata-plugin-pythond = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_GO)
  list(APPEND _rpm_main_requires "netdata-plugin-go = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_DEBUGFS)
  list(APPEND _rpm_main_requires "netdata-plugin-debugfs = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_CHARTS)
  list(APPEND _rpm_main_requires "netdata-plugin-chartsd = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_SLABINFO)
  list(APPEND _rpm_main_requires "netdata-plugin-slabinfo = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_PERF)
  list(APPEND _rpm_main_requires "netdata-plugin-perf = ${NETDATA_RPM_VERSION}")
endif()
if(ENABLE_PLUGIN_NFACCT)
  list(APPEND _rpm_main_requires "netdata-plugin-nfacct = ${NETDATA_RPM_VERSION}")
endif()

if(NETDATA_RPM_HAVE_WEAK_DEPS)
  if(ENABLE_PLUGIN_IBM)
    list(APPEND _rpm_main_suggests "netdata-plugin-ibm = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_FREEIPMI)
    list(APPEND _rpm_main_suggests "netdata-plugin-freeipmi = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_CUPS)
    list(APPEND _rpm_main_suggests "netdata-plugin-cups = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
    list(APPEND _rpm_main_recommends "netdata-plugin-systemd-journal = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_SYSTEMD_UNITS)
    list(APPEND _rpm_main_recommends "netdata-plugin-systemd-units = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_NETWORK_VIEWER)
    list(APPEND _rpm_main_recommends "netdata-plugin-network-viewer = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_SCRIPTS)
    list(APPEND _rpm_main_recommends "netdata-plugin-scripts = ${NETDATA_RPM_VERSION}")
  endif()
else()
  if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
    list(APPEND _rpm_main_requires "netdata-plugin-systemd-journal = ${NETDATA_RPM_VERSION}")
  endif()
  if(ENABLE_PLUGIN_NETWORK_VIEWER)
    list(APPEND _rpm_main_requires "netdata-plugin-network-viewer = ${NETDATA_RPM_VERSION}")
  endif()
endif()

list(JOIN _rpm_main_requires ", " CPACK_RPM_NETDATA_PACKAGE_REQUIRES)
if(_rpm_main_recommends)
  list(JOIN _rpm_main_recommends ", " CPACK_RPM_NETDATA_PACKAGE_RECOMMENDS)
endif()
if(_rpm_main_suggests)
  list(JOIN _rpm_main_suggests ", " CPACK_RPM_NETDATA_PACKAGE_SUGGESTS)
endif()

# File attributes that differ from the staged tree (which is root:root with
# install-rule modes), matching the spec's %files for the main package.
set(CPACK_RPM_NETDATA_USER_FILELIST
    "%dir /usr/libexec/netdata"
    "%dir /usr/libexec/netdata/plugins.d"
    "%dir /usr/lib/netdata"
    "%dir /usr/lib/netdata/conf.d"
    "%config(noreplace) /etc/netdata/netdata.conf"
    "%config(noreplace) /etc/netdata/netdata-updater.conf"
    "%config(noreplace) /etc/logrotate.d/netdata"
    "%attr(0644,root,netdata) /etc/netdata/.install-type"
    "%attr(0755,root,netdata) /etc/netdata/edit-config"
    "%attr(0750,root,netdata) /usr/libexec/netdata/install-service.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/netdata-uninstaller.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/netdata-updater.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/alarm-notify.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/anonymous-statistics.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/cgroup-name"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/cgroup-network-helper.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/get-kubernetes-labels.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/ioping.plugin"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/loopsleepms.sh.inc"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/system-info.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/tc-qos-helper.sh"
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/cgroup-network"
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/local-listeners"
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/ndsudo"
    "%attr(0770,netdata,netdata) %dir /var/cache/netdata"
    "%attr(0770,netdata,netdata) %dir /var/lib/netdata"
    "%attr(0770,netdata,netdata) %dir /var/lib/netdata/cloud.d"
    "%attr(0770,netdata,netdata) %dir /var/lib/netdata/registry"
    "%attr(0755,netdata,root) %dir /var/log/netdata")

if(NETDATA_RPM_DOC_DIR)
  list(APPEND CPACK_RPM_NETDATA_USER_FILELIST
       "%doc /${NETDATA_RPM_DOC_DIR}/README.md")
endif()

#
# user
#

set(CPACK_COMPONENT_USER_DESCRIPTION
    "User and group accounts for the Netdata Agent")

set(CPACK_DEBIAN_USER_PACKAGE_NAME "netdata-user")
set(CPACK_DEBIAN_USER_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_USER_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_USER_PACKAGE_DEPENDS "adduser | systemd")

set(CPACK_DEBIAN_USER_PACKAGE_CONTROL_EXTRA
    "${PKG_FILES_PATH}/deb/user/postinst")

set(CPACK_DEBIAN_USER_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_USER_PACKAGE_NAME "netdata-user")
set(CPACK_RPM_USER_PACKAGE_SUMMARY "User and group accounts for the Netdata Agent")
if(NETDATA_RPM_HAVE_SYSUSER)
  set(CPACK_RPM_USER_PACKAGE_REQUIRES "systemd")
  if(NETDATA_DISTRO_SUSE)
    set(CPACK_RPM_USER_PACKAGE_PROVIDES "user(netdata), group(netdata)")
  endif()
else()
  set(CPACK_RPM_USER_PACKAGE_REQUIRES "/usr/sbin/useradd, /usr/sbin/groupadd")
endif()

#
# dashboard
#

set(CPACK_COMPONENT_DASHBOARD_DEPENDS "netdata")
set(CPACK_COMPONENT_DASHBOARD_DESCRIPTION
    "The local dashboard for the Netdata Agent.
 This allows access to the dashboard on the local node without internet access.")

set(CPACK_DEBIAN_DASHBOARD_PACKAGE_NAME "netdata-dashboard")
set(CPACK_DEBIAN_DASHBOARD_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_DASHBOARD_PACKAGE_CONFLICTS "netdata (<< ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_DASHBOARD_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_DASHBOARD_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_DASHBOARD_PACKAGE_NAME "netdata-dashboard")
set(CPACK_RPM_DASHBOARD_PACKAGE_SUMMARY "The local dashboard for the Netdata Agent")
set(CPACK_RPM_DASHBOARD_PACKAGE_REQUIRES "netdata >= ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_DASHBOARD_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_DASHBOARD_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_DASHBOARD_USER_FILELIST
    "%dir /usr/share/netdata/web")

#
# apps.plugin
#

set(CPACK_COMPONENT_PLUGIN-APPS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-APPS_DESCRIPTION
		"The per-application metrics collector plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect per-application and
 per-user metrics without using cgroups.")

set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_NAME "netdata-plugin-apps")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-APPS_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-apps/postinst")

set(CPACK_DEBIAN_PLUGIN-APPS_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-APPS_PACKAGE_NAME "netdata-plugin-apps")
set(CPACK_RPM_PLUGIN-APPS_PACKAGE_SUMMARY "The per-application metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-APPS_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-APPS_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-APPS_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-APPS_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-APPS_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-APPS_USER_FILELIST
    "%attr(0750,root,netdata) %caps(cap_dac_read_search,cap_sys_ptrace=ep) /usr/libexec/netdata/plugins.d/apps.plugin")

#
# charts.d.plugin
#

set(CPACK_COMPONENT_PLUGIN-CHARTSD_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-CHARTSD_DESCRIPTION
		"The charts.d metrics collection plugin for the Netdata Agent
 This plugin adds a selection of additional collectors written in shell
 script to the Netdata Agent. It includes collectors for APCUPSD,
 LibreSWAN, OpenSIPS, and Wireless access point statistics.")

set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_NAME "netdata-plugin-chartsd")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_PREDEPENDS "netdata-user")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_DEPENDS "bash")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_ARCHITECTURE "all")
set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_SUGGESTS "apcupsd, iw, sudo")

set(CPACK_DEBIAN_PLUGIN-CHARTSD_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-chartsd/postinst")

set(CPACK_DEBIAN_PLUGIN-CHARTSD_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-CHARTSD_PACKAGE_NAME "netdata-plugin-chartsd")
set(CPACK_RPM_PLUGIN-CHARTSD_PACKAGE_SUMMARY "The charts.d metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-CHARTSD_PACKAGE_REQUIRES "bash, netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-CHARTSD_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_HAVE_WEAK_DEPS)
  set(CPACK_RPM_PLUGIN-CHARTSD_PACKAGE_SUGGESTS "apcupsd, iw, sudo")
endif()
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-CHARTSD_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-CHARTSD_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-CHARTSD_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-CHARTSD_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/charts.d.plugin"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/charts.d.dryrun-helper.sh"
    "%attr(0750,root,netdata) %dir /usr/libexec/netdata/charts.d"
    "%attr(0750,root,netdata) /usr/libexec/netdata/charts.d/example.chart.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/charts.d/libreswan.chart.sh"
    "%attr(0750,root,netdata) /usr/libexec/netdata/charts.d/opensips.chart.sh")

#
# cups.plugin
#

set(CPACK_COMPONENT_PLUGIN-CUPS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-CUPS_DESCRIPTION
	  "The CUPS metrics collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics from the Common UNIX Printing System.")

set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_NAME "netdata-plugin-cups")
set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_PREDEPENDS "netdata-user")
set(CPACK_DEBIAN_PLUGIN-CUPS_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-cups/postinst")

set(CPACK_DEBIAN_PLUGIN-CUPS_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-CUPS_PACKAGE_NAME "netdata-plugin-cups")
set(CPACK_RPM_PLUGIN-CUPS_PACKAGE_SUMMARY "The CUPS metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-CUPS_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-CUPS_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-CUPS_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-CUPS_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-CUPS_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/cups.plugin")

#
# debugfs.plugin
#

set(CPACK_COMPONENT_PLUGIN-DEBUGFS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-DEBUGFS_DESCRIPTION
		"The debugfs metrics collector for the Netdata Agent
 This plugin allows the Netdata Agent to collect Linux kernel metrics
 exposed through debugfs.")

set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_NAME "netdata-plugin-debugfs")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_PREDEPENDS "libcap2-bin, adduser")
set(CPACK_DEBIAN_PLUGIN-DEBUGFS_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-debugfs/postinst")

set(CPACK_DEBIAN_PLUGIN-DEBUGFS_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-DEBUGFS_PACKAGE_NAME "netdata-plugin-debugfs")
set(CPACK_RPM_PLUGIN-DEBUGFS_PACKAGE_SUMMARY "The debugfs metrics collector for the Netdata Agent")
set(CPACK_RPM_PLUGIN-DEBUGFS_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-DEBUGFS_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-DEBUGFS_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-DEBUGFS_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-DEBUGFS_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-DEBUGFS_USER_FILELIST
    "%attr(0750,root,netdata) %caps(cap_dac_read_search,cap_audit_control=ep) /usr/libexec/netdata/plugins.d/debugfs.plugin")

#
# ebpf.plugin
#

set(CPACK_COMPONENT_PLUGIN-EBPF_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-EBPF_DESCRIPTION
		"The eBPF metrics collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to use eBPF code to collect more
 detailed kernel-level metrics for the system.")

set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_NAME "netdata-plugin-ebpf")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_PREDEPENDS "netdata-user")
set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_RECOMMENDS "netdata-plugin-apps (= ${CPACK_PACKAGE_VERSION}), netdata-ebpf-code-legacy (= ${CPACK_PACKAGE_VERSION})")

set(CPACK_DEBIAN_PLUGIN-EBPF_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-ebpf/postinst")

set(CPACK_DEBIAN_PLUGIN-EBPF_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-EBPF_PACKAGE_NAME "netdata-plugin-ebpf")
set(CPACK_RPM_PLUGIN-EBPF_PACKAGE_SUMMARY "The eBPF metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-EBPF_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-EBPF_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_HAVE_WEAK_DEPS)
  set(CPACK_RPM_PLUGIN-EBPF_PACKAGE_RECOMMENDS
      "netdata-plugin-apps = ${NETDATA_RPM_VERSION}, netdata-ebpf-legacy-code >= ${NETDATA_RPM_VERSION}")
else()
  string(APPEND CPACK_RPM_PLUGIN-EBPF_PACKAGE_REQUIRES
         ", netdata-plugin-apps = ${NETDATA_RPM_VERSION}, netdata-ebpf-legacy-code >= ${NETDATA_RPM_VERSION}")
endif()
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-EBPF_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-EBPF_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-EBPF_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-EBPF_USER_FILELIST
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/ebpf.plugin")
if(ENABLE_PLUGIN_EBPF_GO)
  list(APPEND CPACK_RPM_PLUGIN-EBPF_USER_FILELIST
       "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/ebpf-go.plugin")
endif()

#
# ebpf-code-legacy
#

set(CPACK_COMPONENT_EBPF-CODE-LEGACY_DEPENDS "netdata")
set(CPACK_COMPONENT_EBPF-CODE-LEGACY_DESCRIPTION
		"Compiled eBPF legacy code for the Netdata eBPF plugin
 This package provides the pre-compiled eBPF legacy code for use by
 the Netdata eBPF plugin.  This code is only needed when using the eBPF
 plugin with kernel that do not include BTF support (mostly kernel
 versions lower than 5.10).")

set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_NAME "netdata-ebpf-code-legacy")
set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_PREDEPENDS "netdata-user")
set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_RECOMMENDS  "netdata-plugin-ebpf (= ${CPACK_PACKAGE_VERSION})")

set(CPACK_DEBIAN_EBPF-CODE-LEGACY_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/ebpf-code-legacy/postinst")

set(CPACK_DEBIAN_EBPF-CODE-LEGACY_DEBUGINFO_PACKAGE Off)

# The RPM package name predates the DEB one and is reversed relative to it;
# both are shipped public names and must stay as they are.
set(CPACK_RPM_EBPF-CODE-LEGACY_PACKAGE_NAME "netdata-ebpf-legacy-code")
set(CPACK_RPM_EBPF-CODE-LEGACY_PACKAGE_SUMMARY "Compiled eBPF legacy code for the Netdata eBPF plugin")
set(CPACK_RPM_EBPF-CODE-LEGACY_PACKAGE_REQUIRES "netdata-plugin-ebpf = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_EBPF-CODE-LEGACY_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_EBPF-CODE-LEGACY_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_EBPF-CODE-LEGACY_DEFAULT_USER "root")
set(CPACK_RPM_EBPF-CODE-LEGACY_DEFAULT_GROUP "netdata")
set(CPACK_RPM_EBPF-CODE-LEGACY_DEFAULT_FILE_PERMISSIONS
    OWNER_READ OWNER_WRITE GROUP_READ)

#
# freeipmi.plugin
#

set(CPACK_COMPONENT_PLUGIN-FREEIPMI_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-FREEIPMI_DESCRIPTION
		"The FreeIPMI metrics collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics from hardware
 using FreeIPMI.")

set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_NAME "netdata-plugin-freeipmi")
set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-FREEIPMI_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-freeipmi/postinst")

set(CPACK_DEBIAN_PLUGIN-FREEIPMI_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-FREEIPMI_PACKAGE_NAME "netdata-plugin-freeipmi")
set(CPACK_RPM_PLUGIN-FREEIPMI_PACKAGE_SUMMARY "The FreeIPMI metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-FREEIPMI_PACKAGE_REQUIRES "freeipmi, netdata = ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-FREEIPMI_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-FREEIPMI_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-FREEIPMI_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-FREEIPMI_USER_FILELIST
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/freeipmi.plugin")

#
# go.plugin
#

set(CPACK_COMPONENT_PLUGIN-GO_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-GO_DESCRIPTION
		"The go.d metrics collection plugin for the Netdata Agent
 This plugin adds a selection of additional collectors written in Go to
 the Netdata Agent. A significant percentage of the application specific
 collectors provided by Netdata are part of this plugin, so most users
 will want it installed.")

set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_NAME "netdata-plugin-go")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_PREDEPENDS "libcap2-bin, adduser")
set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_SUGGESTS "nvme-cli")

set(CPACK_DEBIAN_PLUGIN-GO_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-go/postinst")

set(CPACK_DEBIAN_PLUGIN-GO_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-GO_PACKAGE_NAME "netdata-plugin-go")
set(CPACK_RPM_PLUGIN-GO_PACKAGE_SUMMARY "The go.d metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-GO_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-GO_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_HAVE_WEAK_DEPS)
  set(CPACK_RPM_PLUGIN-GO_PACKAGE_SUGGESTS "nvme-cli, sudo")
endif()
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-GO_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-GO_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-GO_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-GO_USER_FILELIST
    "%attr(0750,root,netdata) %caps(cap_dac_read_search,cap_net_admin,cap_net_raw,cap_net_bind_service=eip) /usr/libexec/netdata/plugins.d/go.d.plugin"
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/snmp-trap-profile-gen")

#
# scripts.d.plugin
#

set(CPACK_COMPONENT_PLUGIN-SCRIPTS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-SCRIPTS_DESCRIPTION
		"The scripts metrics collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics using scripts
that provide data in an extended version of the output format used by
Nagios plugins. This provides compatibility with most Nagios plugins,
as well as enabling simple active checks.")

set(CPACK_DEBIAN_PLUGIN-SCRIPTS_PACKAGE_NAME "netdata-plugin-scripts")
set(CPACK_DEBIAN_PLUGIN-SCRIPTS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-SCRIPTS_PACKAGE_CONFLICTS "netdata (<< 2.8)")
set(CPACK_DEBIAN_PLUGIN-SCRIPTS_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-SCRIPTS_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-scripts/postinst")

set(CPACK_DEBIAN_PLUGIN-SCRIPTS_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-SCRIPTS_PACKAGE_NAME "netdata-plugin-scripts")
set(CPACK_RPM_PLUGIN-SCRIPTS_PACKAGE_SUMMARY "The scripts metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-SCRIPTS_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-SCRIPTS_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-SCRIPTS_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-SCRIPTS_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-SCRIPTS_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-SCRIPTS_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/scripts.d.plugin")

#
# ibm.plugin
#

set(CPACK_COMPONENT_PLUGIN-IBM_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-IBM_DESCRIPTION
		"The IBM ecosystem metrics collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics from IBM
 systems including AS/400 (IBM i), DB2 databases, MQ queues, and WebSphere
 application servers. Database collectors use unixODBC and require appropriate
 ODBC drivers.")

set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_NAME "netdata-plugin-ibm")
set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_PREDEPENDS "netdata-user")
set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_DEPENDS "unixodbc, netdata-plugin-ibm-libs (= ${CPACK_PACKAGE_VERSION})")
set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_SUGGESTS "libxml2")

set(CPACK_DEBIAN_PLUGIN-IBM_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-ibm/postinst")

set(CPACK_DEBIAN_PLUGIN-IBM_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-IBM_PACKAGE_NAME "netdata-plugin-ibm")
set(CPACK_RPM_PLUGIN-IBM_PACKAGE_SUMMARY "The IBM ecosystem metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-IBM_PACKAGE_REQUIRES
    "netdata = ${NETDATA_RPM_VERSION}, netdata-plugin-ibm-libs = ${NETDATA_RPM_VERSION}, unixODBC")
set(CPACK_RPM_PLUGIN-IBM_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
# The vendored IBM MQ client must not generate automatic provides/requires.
set(CPACK_RPM_PLUGIN-IBM_PACKAGE_AUTOREQPROV "no")
if(NETDATA_RPM_HAVE_SYSUSER)
  set(CPACK_RPM_PLUGIN-IBM_PACKAGE_REQUIRES_PRE "user(netdata), group(netdata)")
elseif(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-IBM_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-IBM_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-IBM_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-IBM_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/ibm.d.plugin")

set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_DESCRIPTION
		"IBM MQ client libraries for the Netdata IBM ecosystem metrics collection plugin.
 This package provides the IBM MQ client libraries needed by Netdata IBM
 ecosystem metrics collection plugin.")

set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_PACKAGE_NAME "netdata-plugin-ibm-libs")
set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_DEBUGINFO_PACKAGE Off)
set(CPACK_DEBIAN_PLUGIN-IBM-LIBS_PACKAGE_SHLIBDEPS Off)

set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_NAME "netdata-plugin-ibm-libs")
set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_SUMMARY "IBM MQ client libraries for the Netdata IBM ecosystem metrics collection plugin.")
set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_AUTOREQPROV "no")
if(NETDATA_RPM_HAVE_SYSUSER)
  set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_REQUIRES_PRE "user(netdata), group(netdata)")
elseif(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-IBM-LIBS_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-IBM-LIBS_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-IBM-LIBS_DEFAULT_GROUP "netdata")
# 0640 for the vendored MQ data files; the executable/library exceptions are
# derived from the MQ manifest by install_ibm_runtime.
set(CPACK_RPM_PLUGIN-IBM-LIBS_DEFAULT_FILE_PERMISSIONS
    OWNER_READ OWNER_WRITE GROUP_READ)
if(NETDATA_IBM_MQ_RPM_FILELIST)
  set(CPACK_RPM_PLUGIN-IBM-LIBS_USER_FILELIST ${NETDATA_IBM_MQ_RPM_FILELIST})
endif()

#
# network-viewer.plugin
#

set(CPACK_COMPONENT_PLUGIN-NETWORK-VIEWER_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-NETWORK-VIEWER_DESCRIPTION
		"The network viewer plugin for the Netdata Agent
 This plugin allows the Netdata Agent to provide network connection
 mapping functionality for use in Netdata Cloud.")

set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_NAME "netdata-plugin-network-viewer")
set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_PREDEPENDS "libcap2-bin, adduser")
set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_RECOMMENDS "netdata-plugin-ebpf (= ${CPACK_PACKAGE_VERSION})")

set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-network-viewer/postinst")

set(CPACK_DEBIAN_PLUGIN-NETWORK-VIEWER_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_NAME "netdata-plugin-network-viewer")
set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_SUMMARY "The network viewer plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(ENABLE_PLUGIN_EBPF)
  if(NETDATA_RPM_HAVE_WEAK_DEPS)
    set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_RECOMMENDS "netdata-plugin-ebpf = ${NETDATA_RPM_VERSION}")
  else()
    string(APPEND CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_REQUIRES
           ", netdata-plugin-ebpf = ${NETDATA_RPM_VERSION}")
  endif()
endif()
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-NETWORK-VIEWER_USER_FILELIST
    "%attr(0750,root,netdata) %caps(cap_sys_admin,cap_sys_ptrace,cap_dac_read_search=ep) /usr/libexec/netdata/plugins.d/network-viewer.plugin")

#
# otel.plugin
#

set(CPACK_COMPONENT_PLUGIN-OTEL_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-OTEL_DESCRIPTION
		"The OpenTelemetry collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics and logs via
 OpenTelemetry gRPC protocol, providing integration with modern observability
 stacks.")

set(CPACK_DEBIAN_PLUGIN-OTEL_PACKAGE_NAME "netdata-plugin-otel")
set(CPACK_DEBIAN_PLUGIN-OTEL_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-OTEL_PACKAGE_CONFLICTS "netdata (<< 1.40), netdata-plugin-otel-signal-viewer")
set(CPACK_DEBIAN_PLUGIN-OTEL_PACKAGE_REPLACES "netdata-plugin-otel-signal-viewer")
set(CPACK_DEBIAN_PLUGIN-OTEL_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-OTEL_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-otel/postinst")

set(CPACK_DEBIAN_PLUGIN-OTEL_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-OTEL_PACKAGE_NAME "netdata-plugin-otel")
set(CPACK_RPM_PLUGIN-OTEL_PACKAGE_SUMMARY "The Open Telemetry plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-OTEL_PACKAGE_REQUIRES "netdata >= ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-OTEL_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-OTEL_PACKAGE_OBSOLETES "netdata-plugin-otel-signal-viewer")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-OTEL_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-OTEL_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-OTEL_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-OTEL_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/otel-plugin")

#
# netflow.plugin
#

set(CPACK_COMPONENT_PLUGIN-NETFLOW_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-NETFLOW_DESCRIPTION
		"The NetFlow/IPFIX/sFlow flow analysis plugin for the Netdata Agent
 This plugin ingests, stores, and serves NetFlow/IPFIX/sFlow flow data for
 traffic analysis functions.")

set(CPACK_DEBIAN_PLUGIN-NETFLOW_PACKAGE_NAME "netdata-plugin-netflow")
set(CPACK_DEBIAN_PLUGIN-NETFLOW_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-NETFLOW_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-NETFLOW_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-netflow/postinst")

set(CPACK_DEBIAN_PLUGIN-NETFLOW_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-NETFLOW_PACKAGE_NAME "netdata-plugin-netflow")
set(CPACK_RPM_PLUGIN-NETFLOW_PACKAGE_SUMMARY "The NetFlow/IPFIX/sFlow flow analysis plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-NETFLOW_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-NETFLOW_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-NETFLOW_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-NETFLOW_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-NETFLOW_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-NETFLOW_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/netflow-plugin"
    "%attr(0750,root,netdata) /usr/sbin/topology-ip-intel-downloader")

#
# nfacct.plugin
#

set(CPACK_COMPONENT_PLUGIN-NFACCT_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-NFACCT_DESCRIPTION
		"The NFACCT metrics collection plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics from the firewall
 using NFACCT objects.")

set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_NAME "netdata-plugin-nfacct")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-NFACCT_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-nfacct/postinst")

set(CPACK_DEBIAN_PLUGIN-NFACCT_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-NFACCT_PACKAGE_NAME "netdata-plugin-nfacct")
set(CPACK_RPM_PLUGIN-NFACCT_PACKAGE_SUMMARY "The NFACCT metrics collection plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-NFACCT_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-NFACCT_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-NFACCT_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-NFACCT_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-NFACCT_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-NFACCT_USER_FILELIST
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/nfacct.plugin")

#
# perf.plugin
#

set(CPACK_COMPONENT_PLUGIN-PERF_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-PERF_DESCRIPTION
		"The perf metrics collector for the Netdata Agent
 This plugin allows the Netdata to collect metrics from the Linux perf
 subsystem.")

set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_NAME "netdata-plugin-perf")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-PERF_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-perf/postinst")

set(CPACK_DEBIAN_PLUGIN-PERF_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-PERF_PACKAGE_NAME "netdata-plugin-perf")
set(CPACK_RPM_PLUGIN-PERF_PACKAGE_SUMMARY "The perf metrics collector for the Netdata Agent")
set(CPACK_RPM_PLUGIN-PERF_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-PERF_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-PERF_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-PERF_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-PERF_DEFAULT_GROUP "netdata")
# cap_perfmon exists on EL >= 9 and Fedora >= 36 (kernel 5.8+); older
# targets fall back to cap_sys_admin, as the spec does.
if((NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR GREATER_EQUAL 9) OR
   (NETDATA_DISTRO_FEDORA AND NETDATA_DISTRO_VERSION_MAJOR GREATER_EQUAL 36))
  set(CPACK_RPM_PLUGIN-PERF_USER_FILELIST
      "%attr(0750,root,netdata) %caps(cap_perfmon=ep) /usr/libexec/netdata/plugins.d/perf.plugin")
else()
  set(CPACK_RPM_PLUGIN-PERF_USER_FILELIST
      "%attr(0750,root,netdata) %caps(cap_sys_admin=ep) /usr/libexec/netdata/plugins.d/perf.plugin")
endif()

#
# pythond.plugin
#

set(CPACK_COMPONENT_PLUGIN-PYTHOND_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-PYTHOND_DESCRIPTION
		"The python.d metrics collection plugin for the Netdata Agent
 Many of the collectors provided by this package are also available
 in netdata-plugin-god. In msot cases, you probably want to use those
 versions instead of the Python versions.")

set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_NAME "netdata-plugin-pythond")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_PREDEPENDS "netdata-user")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_SUGGESTS "sudo")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACHAGE_DEPENDS "python3")
set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_ARCHITECTURE "all")

set(CPACK_DEBIAN_PLUGIN-PYTHOND_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-pythond/postinst")

set(CPACK_DEBIAN_PLUGIN-PYTHOND_DEBUGINFO_PACKAGE Off)

set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_NAME "netdata-plugin-pythond")
set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_SUMMARY "The python.d metrics collection plugin for the Netdata Agent")
if(NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR LESS 8)
  set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}, python")
else()
  set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}, python3")
endif()
set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_HAVE_WEAK_DEPS)
  set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_SUGGESTS "sudo")
endif()
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-PYTHOND_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-PYTHOND_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-PYTHOND_DEFAULT_GROUP "netdata")
# The spec's %defattr makes the whole python.d tree 0750; the stock configs
# keep 0644 with a world-readable directory.
set(CPACK_RPM_PLUGIN-PYTHOND_DEFAULT_FILE_PERMISSIONS
    OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE)
set(CPACK_RPM_PLUGIN-PYTHOND_DEFAULT_DIR_PERMISSIONS
    OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE)
set(CPACK_RPM_PLUGIN-PYTHOND_USER_FILELIST
    "%attr(0644,root,netdata) /usr/lib/netdata/conf.d/python.d.conf"
    "%attr(0755,root,netdata) %dir /usr/lib/netdata/conf.d/python.d"
    "%attr(0644,root,netdata) /usr/lib/netdata/conf.d/python.d/am2320.conf"
    "%attr(0644,root,netdata) /usr/lib/netdata/conf.d/python.d/go_expvar.conf"
    "%attr(0644,root,netdata) /usr/lib/netdata/conf.d/python.d/haproxy.conf"
    "%attr(0644,root,netdata) /usr/lib/netdata/conf.d/python.d/pandas.conf"
    "%attr(0644,root,netdata) /usr/lib/netdata/conf.d/python.d/traefik.conf")

#
# slabinfo.plugin
#

set(CPACK_COMPONENT_PLUGIN-SLABINFO_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-SLABINFO_DESCRIPTION
		"The slabinfo metrics collector for the Netdata Agent
 This plugin allows the Netdata Agent to collect perfromance and
 utilization metrics for the Linux kernel’s SLAB allocator.")

set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_NAME "netdata-plugin-slabinfo")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-SLABINFO_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-slabinfo/postinst")

set(CPACK_DEBIAN_PLUGIN-SLABINFO-DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-SLABINFO_PACKAGE_NAME "netdata-plugin-slabinfo")
set(CPACK_RPM_PLUGIN-SLABINFO_PACKAGE_SUMMARY "The slabinfo metrics collector for the Netdata Agent")
set(CPACK_RPM_PLUGIN-SLABINFO_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-SLABINFO_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-SLABINFO_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-SLABINFO_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-SLABINFO_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-SLABINFO_USER_FILELIST
    "%attr(0750,root,netdata) %caps(cap_dac_read_search=ep) /usr/libexec/netdata/plugins.d/slabinfo.plugin")

#
# systemd-journal.plugin
#

set(CPACK_COMPONENT_PLUGIN-SYSTEMD-JOURNAL_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-SYSTEMD-JOURNAL_DESCRIPTION
		"The systemd-journal collector for the Netdata Agent
 This plugin allows the Netdata Agent to present logs from the systemd
 journal on Netdata Cloud or the local Agent dashboard.")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_NAME "netdata-plugin-systemd-journal")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_PREDEPENDS "libcap2-bin, adduser")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-systemd-journal/postinst")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-JOURNAL_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_NAME "netdata-plugin-systemd-journal")
set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_SUMMARY "The systemd-journal plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-SYSTEMD-JOURNAL_USER_FILELIST
    "%attr(0750,root,netdata) %caps(cap_dac_read_search=ep) /usr/libexec/netdata/plugins.d/systemd-journal.plugin")

set(CPACK_COMPONENT_PLUGIN-JOURNAL-VIEWER_DESCRIPTION
		"Transitional dummy package.
 This package simply ensures a clean upgrade to the renamed
 netdata-plugin-systemd-journal package. Once that package is installed,
 you can safely remove this one.")

set(CPACK_DEBIAN_PLUGIN-JOURNAL-VIEWER_PACKAGE_NAME "netdata-plugin-journal-viewer")
set(CPACK_DEBIAN_PLUGIN-JOURNAL-VIEWER_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-JOURNAL-VIEWER_DEPENDS "netdata-plugin-systemd-journal (= ${CPACK_PACKAGE_VERSION})")

set(CPACK_RPM_PLUGIN-JOURNAL-VIEWER_PACKAGE_NAME "netdata-plugin-journal-viewer")
set(CPACK_RPM_PLUGIN-JOURNAL-VIEWER_PACKAGE_SUMMARY "Transitional dummy package")
set(CPACK_RPM_PLUGIN-JOURNAL-VIEWER_PACKAGE_REQUIRES "netdata-plugin-systemd-journal")

#
# systemd-units.plugin
#

set(CPACK_COMPONENT_PLUGIN-SYSTEMD-UNITS_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-SYSTEMD-UNITS_DESCRIPTION
		"The systemd-units collector for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics about systmed units.")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-UNITS_PACKAGE_NAME "netdata-plugin-systemd-units")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-UNITS_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-SYSTEMD-UNITS_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD-UNITS_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-systemd-units/postinst")

set(CPACK_DEBIAN_PLUGIN-SYSTEMD_UNITS_DEBUGINFO_PACKAGE On)

set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_PACKAGE_NAME "netdata-plugin-systemd-units")
set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_PACKAGE_SUMMARY "The systemd units plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-SYSTEMD-UNITS_USER_FILELIST
    "%attr(0750,root,netdata) /usr/libexec/netdata/plugins.d/systemd-units.plugin")

#
# xenstat.plugin
#

set(CPACK_COMPONENT_PLUGIN-XENSTAT_DEPENDS "netdata")
set(CPACK_COMPONENT_PLUGIN-XENSTAT_DESCRIPTION
		"The xenstat plugin for the Netdata Agent
 This plugin allows the Netdata Agent to collect metrics from the Xen
 Hypervisor.")

set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_NAME "netdata-plugin-xenstat")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_SECTION "net")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_CONFLICTS "netdata (<< 1.40)")
set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_PREDEPENDS "netdata-user")

set(CPACK_DEBIAN_PLUGIN-XENSTAT_PACKAGE_CONTROL_EXTRA
	  "${PKG_FILES_PATH}/deb/plugin-xenstat/postinst")

set(CPACK_DEBIAN_PLUGIN-XENSTAT_DEBUGINFO_PACKAGE On)

# The spec's xenstat gating is dead code (a typo makes it always disabled),
# so no RPM ever ships this package today; the configuration exists for the
# day ENABLE_PLUGIN_XENSTAT is turned on for RPM builds.
set(CPACK_RPM_PLUGIN-XENSTAT_PACKAGE_NAME "netdata-plugin-xenstat")
set(CPACK_RPM_PLUGIN-XENSTAT_PACKAGE_SUMMARY "The xenstat plugin for the Netdata Agent")
set(CPACK_RPM_PLUGIN-XENSTAT_PACKAGE_REQUIRES "netdata = ${NETDATA_RPM_VERSION}")
set(CPACK_RPM_PLUGIN-XENSTAT_PACKAGE_CONFLICTS "netdata < ${NETDATA_RPM_VERSION}")
if(NETDATA_RPM_USER_PREDEP)
  set(CPACK_RPM_PLUGIN-XENSTAT_PACKAGE_REQUIRES_PRE "${NETDATA_RPM_USER_PREDEP}")
endif()
set(CPACK_RPM_PLUGIN-XENSTAT_DEFAULT_USER "root")
set(CPACK_RPM_PLUGIN-XENSTAT_DEFAULT_GROUP "netdata")
set(CPACK_RPM_PLUGIN-XENSTAT_USER_FILELIST
    "%attr(4750,root,netdata) /usr/libexec/netdata/plugins.d/xenstat.plugin")

#
# CPack components
#

list(APPEND CPACK_COMPONENTS_ALL "netdata")
list(APPEND CPACK_COMPONENTS_ALL "user")
if(ENABLE_DASHBOARD)
  list(APPEND CPACK_COMPONENTS_ALL "dashboard")
endif()
if(ENABLE_PLUGIN_APPS)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-apps")
endif()
if(ENABLE_PLUGIN_CHARTS)
  list(APPEND CPACK_COMPONENTS_ALL "plugin-chartsd")
endif()
if(ENABLE_PLUGIN_CUPS)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-cups")
endif()
if(ENABLE_PLUGIN_DEBUGFS)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-debugfs")
endif()
if(ENABLE_PLUGIN_EBPF)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-ebpf")
endif()
if(ENABLE_LEGACY_EBPF_PROGRAMS)
        list(APPEND CPACK_COMPONENTS_ALL "ebpf-code-legacy")
endif()
if(ENABLE_PLUGIN_FREEIPMI)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-freeipmi")
endif()
if(ENABLE_PLUGIN_GO)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-go")
endif()
if(ENABLE_PLUGIN_IBM)
  list(APPEND CPACK_COMPONENTS_ALL "plugin-ibm")
  list(APPEND CPACK_COMPONENTS_ALL "plugin-ibm-libs")
endif()
if(ENABLE_PLUGIN_SCRIPTS)
  list(APPEND CPACK_COMPONENTS_ALL "plugin-scripts")
endif()
if(ENABLE_PLUGIN_NETWORK_VIEWER)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-network-viewer")
endif()
if(ENABLE_PLUGIN_NFACCT)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-nfacct")
endif()
if(ENABLE_PLUGIN_NETFLOW)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-netflow")
endif()
if(ENABLE_PLUGIN_PERF)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-perf")
endif()
if(ENABLE_PLUGIN_PYTHON)
  list(APPEND CPACK_COMPONENTS_ALL "plugin-pythond")
endif()
if(ENABLE_PLUGIN_SLABINFO)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-slabinfo")
endif()
if(ENABLE_PLUGIN_SYSTEMD_JOURNAL)
  list(APPEND CPACK_COMPONENTS_ALL "plugin-journal-viewer")
  list(APPEND CPACK_COMPONENTS_ALL "plugin-systemd-journal")
endif()
if(ENABLE_PLUGIN_SYSTEMD_UNITS)
  list(APPEND CPACK_COMPONENTS_ALL "plugin-systemd-units")
endif()
if(ENABLE_PLUGIN_XENSTAT)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-xenstat")
endif()
if(ENABLE_PLUGIN_OTEL)
        list(APPEND CPACK_COMPONENTS_ALL "plugin-otel")
endif()

include(CPack)
