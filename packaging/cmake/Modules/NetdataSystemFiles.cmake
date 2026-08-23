# SPDX-License-Identifier: GPL-3.0-or-later
# Generated config header, stock configuration, service and system files, the
# dashboard bundle, and packaging. Relocated verbatim from the tail of the root
# CMakeLists.txt.
#
# include()d rather than add_subdirectory()d so CMAKE_CURRENT_SOURCE_DIR and
# CMAKE_CURRENT_BINARY_DIR keep pointing at the repository and build roots: the
# 27 relative configure_file() outputs and every relative install() path below
# depend on it. Nothing here may use CMAKE_CURRENT_LIST_DIR.
#
# It must stay included at the point it was cut from. The get_directory_property
# calls at the top snapshot the compile and link options accumulated so far, and
# that snapshot reaches config.h and is shipped as build-info-cmake-cache.gz.

include_guard()

#
# Generate config file
#

# Collect all compiler flags
get_directory_property(NETDATA_COMPILE_OPTIONS COMPILE_OPTIONS)
get_directory_property(NETDATA_COMPILE_DEFINITIONS COMPILE_DEFINITIONS)

# Collect all linker flags
get_directory_property(NETDATA_LINK_OPTIONS LINK_OPTIONS)
get_directory_property(NETDATA_LINK_LIBRARIES LINK_LIBRARIES)

list(APPEND CONFIGURE_OPTIONS
        # Build type and languages
        "-DCMAKE_BUILD_TYPE=${CMAKE_BUILD_TYPE}"
        "-DCMAKE_C_STANDARD=${CMAKE_C_STANDARD}"
        "-DCMAKE_CXX_STANDARD=${CMAKE_CXX_STANDARD}"
        "-DBUILD_SHARED_LIBS=${BUILD_SHARED_LIBS}"

        # Compiler flags (dynamically collected)
        "-DCMAKE_C_FLAGS='${CMAKE_C_FLAGS} ${NETDATA_COMPILE_OPTIONS}'"
        "-DCMAKE_CXX_FLAGS='${CMAKE_CXX_FLAGS} ${NETDATA_COMPILE_OPTIONS}'"
        "-DCMAKE_COMPILE_DEFINITIONS='${NETDATA_COMPILE_DEFINITIONS}'"

        # Linker flags (dynamically collected)
        "-DCMAKE_EXE_LINKER_FLAGS='${CMAKE_EXE_LINKER_FLAGS} ${NETDATA_LINK_OPTIONS}'"
        "-DCMAKE_SHARED_LINKER_FLAGS='${CMAKE_SHARED_LINKER_FLAGS}'"
)

string(JOIN " " CONFIGURE_COMMAND "cmake" ${CONFIGURE_OPTIONS})

if (NOT NETDATA_USER)
        set(NETDATA_USER "netdata")
endif()

set(BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME "build-info-cmake-cache.gz")
set(BUILD_INFO_CMAKE_CACHE_ARCHIVE_PATH "usr/share/netdata")

configure_file(packaging/cmake/config.cmake.h.in config.h)

#
# install
#

install(TARGETS netdata COMPONENT netdata DESTINATION "${BINDIR}")

install(DIRECTORY COMPONENT netdata DESTINATION ${CACHE_DEST})
install(DIRECTORY COMPONENT netdata DESTINATION ${LOG_DEST})
install(DIRECTORY COMPONENT netdata DESTINATION ${VARLIB_DEST}/registry)
install(DIRECTORY COMPONENT netdata DESTINATION ${VARLIB_DEST}/cloud.d)
install(DIRECTORY COMPONENT netdata DESTINATION ${RUN_DEST})
install(DIRECTORY COMPONENT netdata DESTINATION ${CONFIG_DEST})
install(DIRECTORY COMPONENT netdata DESTINATION ${CONFIG_DEST}/custom-plugins.d)
install(DIRECTORY COMPONENT netdata DESTINATION ${CONFIG_DEST}/health.d)
install(DIRECTORY COMPONENT netdata DESTINATION ${CONFIG_DEST}/ssl)
install(DIRECTORY COMPONENT netdata DESTINATION ${CONFIG_DEST}/statsd.d)
install(DIRECTORY COMPONENT netdata DESTINATION ${LIBCONFIG_DEST})
install(DIRECTORY COMPONENT netdata DESTINATION ${LIBCONFIG_DEST}/schema.d)
install(DIRECTORY COMPONENT netdata DESTINATION ${PLUGINS_DEST})
install(DIRECTORY COMPONENT netdata DESTINATION ${WEB_DEST})

set(libsysdir_POST "${NETDATA_RUNTIME_PREFIX}/${SYSTEM_DEST}")
set(pkglibexecdir_POST "${NETDATA_RUNTIME_PREFIX}/${LIBEXEC_DEST}")
set(localstatedir_POST "${NETDATA_RUNTIME_PREFIX}/var")
set(sbindir_POST "${NETDATA_BIN_DIR}")
set(configdir_POST "${CONFIG_DIR}")
set(libconfigdir_POST "${LIBCONFIG_DIR}")
set(pluginsdir_POST "${PLUGINS_DIR}")
set(cachedir_POST "${CACHE_DIR}")
set(varlibdir_POST "${VARLIB_DIR}")
set(registrydir_POST "${VARLIB_DIR}/registry")
set(logdir_POST "${LOG_DIR}")
set(netdata_user_POST "${NETDATA_USER}")
set(netdata_group_POST "${NETDATA_USER}")

if(NOT OS_WINDOWS)
        configure_file(src/claim/netdata-claim.sh.in src/claim/netdata-claim.sh @ONLY)
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/src/claim/netdata-claim.sh
                COMPONENT netdata
                DESTINATION "${BINDIR}")
else()
        install(PROGRAMS
                ${CMAKE_BINARY_DIR}/NetdataClaim.exe
                COMPONENT netdata
                DESTINATION "${BINDIR}")
endif()

#
# We don't check ENABLE_PLUGIN_CGROUP_NETWORK because rpm builds assume
# the files exists unconditionally.
#
configure_file(src/collectors/cgroups.plugin/cgroup-network-helper.sh.in
               src/collectors/cgroups.plugin/cgroup-network-helper.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/collectors/cgroups.plugin/cgroup-network-helper.sh
        COMPONENT netdata
        DESTINATION ${PLUGINS_DEST})

if(ENABLE_CGROUP_NAME)
    add_go_target(cgroup-name-target cgroup-name
                  src/collectors/cgroups.plugin/cgroup-name .)
    install(PROGRAMS
            ${CMAKE_BINARY_DIR}/cgroup-name
            COMPONENT netdata
            DESTINATION ${PLUGINS_DEST})
endif()

#
# otel config
#
if(ENABLE_PLUGIN_OTEL)
    configure_file(src/crates/otel-plugin/configs/otel.yaml.in
                   src/crates/otel-plugin/configs/otel.yaml
                   @ONLY)

    install(FILES ${CMAKE_BINARY_DIR}/src/crates/otel-plugin/configs/otel.yaml
            COMPONENT ${NETDATA_OTEL_CONF_COMPONENT}
            DESTINATION ${LIBCONFIG_DEST})
endif()


#
# statsd
#
install(FILES
        src/collectors/statsd.plugin/asterisk.conf
        src/collectors/statsd.plugin/example.conf
        src/collectors/statsd.plugin/k6.conf
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST}/statsd.d)

#
# exporting
#
install(FILES
        src/exporting/exporting.conf
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST})

#
# ioping.plugin
#
install(FILES
        src/collectors/ioping.plugin/ioping.conf
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST})

#
# streaming
#
install(FILES
        src/streaming/stream.conf
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST})

#
# swagger
#
install(FILES
        src/web/api/netdata-swagger.json
        src/web/api/netdata-swagger.yaml
        COMPONENT ${NETDATA_SWAGGER_COMPONENT}
        DESTINATION ${WEB_DEST})

#
# service files
#

# Windows has no POSIX service manager and never runs install-service.sh - the
# script exits 5 on any platform it does not know (system/install-service.sh.in),
# and the Windows service is registered by the installer itself. Staging the
# toolbox there only enlarged the MSI with launchd plists and rc.d scripts.
if(NOT OS_WINDOWS)
  configure_file(system/install-service.sh.in system/install-service.sh @ONLY)
  install(PROGRAMS
          ${CMAKE_BINARY_DIR}/system/install-service.sh
          COMPONENT netdata
          DESTINATION ${LIBEXEC_DEST})

  configure_file(system/launchd/netdata.plist.in system/launchd/netdata.plist @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/launchd/netdata.plist
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/launchd)

  configure_file(system/freebsd/rc.d/netdata.in system/freebsd/rc.d/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/freebsd/rc.d/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/freebsd/rc.d)

  configure_file(system/initd/init.d/netdata.in system/initd/init.d/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/initd/init.d/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/initd/init.d)

  configure_file(system/logrotate/netdata.in system/logrotate/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/logrotate/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/logrotate)
endif()

# The host copy, unlike the one above, lands in logrotate's own directory, so
# only a platform that runs logrotate may receive it. Every other install path
# takes the Netdata-owned copy at install time instead
# (packaging/installer/functions.sh:1061), which is why this is safe to gate.
if(OS_LINUX)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/logrotate/netdata
          COMPONENT netdata
          DESTINATION ${HOST_LOGROTATE_DEST})
endif()

if(NOT OS_WINDOWS)
  configure_file(system/lsb/init.d/netdata.in system/lsb/init.d/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/lsb/init.d/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/lsb/init.d)

  configure_file(system/openrc/conf.d/netdata.in system/openrc/conf.d/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/openrc/conf.d/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/openrc/conf.d)

  configure_file(system/openrc/init.d/netdata.in system/openrc/init.d/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/openrc/init.d/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/openrc/init.d)

  configure_file(system/runit/run.in system/runit/run @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/runit/run
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/runit)

  configure_file(system/dinit/netdata.in system/dinit/netdata @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/dinit/netdata
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/dinit)

  configure_file(system/systemd/netdata.service.in system/systemd/netdata.service @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/systemd/netdata.service
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd)

  install(FILES
          system/systemd/journald@netdata.conf
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd)

  configure_file(system/systemd/netdata.service.v235.in system/systemd/netdata.service.v235 @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/systemd/netdata.service.v235
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd)

  configure_file(system/systemd/sysusers.conf.in system/systemd/sysusers/netdata.conf @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/systemd/sysusers/netdata.conf
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd/sysusers)

  configure_file(system/systemd/tmpfiles.conf.in system/systemd/tmpfiles/netdata.conf @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/systemd/tmpfiles/netdata.conf
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd/tmpfiles)
endif()

# EL 7 and Amazon Linux 2 package the systemd-235-compatible unit variant,
# because their systemd predates the directives the default unit uses.
set(NETDATA_PACKAGED_SYSTEMD_UNIT "${CMAKE_BINARY_DIR}/system/systemd/netdata.service")
if(NETDATA_PACKAGING_FORMAT STREQUAL "rpm" AND
   ((NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR LESS_EQUAL 7) OR
    (NETDATA_DISTRO_AMZN AND NETDATA_DISTRO_VERSION_MAJOR LESS_EQUAL 2)))
        set(NETDATA_PACKAGED_SYSTEMD_UNIT "${CMAKE_BINARY_DIR}/system/systemd/netdata.service.v235")
endif()

if(NETDATA_STAGE_HOST_FILES)
        install(FILES
                ${NETDATA_PACKAGED_SYSTEMD_UNIT}
                COMPONENT netdata
                DESTINATION ${HOST_SYSTEMD_UNIT_DEST}
                RENAME netdata.service)
        install(FILES
                ${CMAKE_BINARY_DIR}/system/systemd/sysusers/netdata.conf
                COMPONENT user
                DESTINATION ${HOST_SYSUSERS_DEST})
        install(FILES
                ${CMAKE_BINARY_DIR}/system/systemd/tmpfiles/netdata.conf
                COMPONENT netdata
                DESTINATION ${HOST_TMPFILES_DEST})
        install(DIRECTORY
                COMPONENT netdata
                DESTINATION ${HOST_JOURNALD_CONF_DEST})
        install(FILES
                system/systemd/journald@netdata.conf
                COMPONENT netdata
                DESTINATION ${HOST_JOURNALD_CONF_DEST}
                RENAME netdata.conf)

        if(NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
                install(FILES
                        system/systemd/50-netdata.preset
                        COMPONENT netdata
                        DESTINATION ${HOST_SYSTEMD_PRESET_DEST})
        endif()
endif()

if(NOT OS_WINDOWS)
  install(FILES
          system/systemd/50-netdata.preset
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd)
endif()

install(FILES
        system/vnodes/vnodes.conf
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST}/vnodes)

install(FILES
        system/.install-type
        COMPONENT netdata
        DESTINATION ${CONFIG_DEST})

install(PROGRAMS
        system/edit-config
        COMPONENT netdata
        DESTINATION ${CONFIG_DEST})

if(BUILD_FOR_PACKAGING)
        set(NETDATA_CONF_DEST "${CONFIG_DEST}")
else()
        set(NETDATA_CONF_DEST "${LIBCONFIG_DEST}")
endif()

#
# misc files
#
if(NETDATA_STAGE_HOST_FILES AND NOT NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
        install(FILES
                ${PKG_FILES_PATH}/deb/netdata/etc/default/netdata
                COMPONENT netdata
                DESTINATION ${HOST_DEFAULT_DEST})

        install(PROGRAMS
                ${PKG_FILES_PATH}/deb/netdata/etc/init.d/netdata
                COMPONENT netdata
                DESTINATION ${HOST_INITD_DEST})
endif()

if(NETDATA_STAGE_HOST_FILES AND NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
        # RPM-only payloads; no other install rule covers them.
        install(PROGRAMS
                packaging/installer/netdata-uninstaller.sh
                COMPONENT netdata
                DESTINATION ${LIBEXEC_DEST}
                PERMISSIONS OWNER_READ OWNER_WRITE OWNER_EXECUTE GROUP_READ GROUP_EXECUTE)

        # %doc README.md equivalent; openSUSE keeps package docs under
        # /usr/share/doc/packages, and the rpm 4.11 era (EL 7, Amazon
        # Linux 2) defaults %_docdir_fmt to %{NAME}-%{VERSION} where newer
        # rpm uses plain %{NAME}.
        if(NETDATA_DISTRO_SUSE)
          set(NETDATA_RPM_DOC_DIR "usr/share/doc/packages/netdata")
        elseif((NETDATA_DISTRO_EL AND NETDATA_DISTRO_VERSION_MAJOR LESS_EQUAL 7) OR
               (NETDATA_DISTRO_AMZN AND NETDATA_DISTRO_VERSION_MAJOR LESS_EQUAL 2))
          string(REPLACE "-" "." _rpm_doc_version "${NETDATA_PACKAGE_VERSION}")
          set(NETDATA_RPM_DOC_DIR "usr/share/doc/netdata-${_rpm_doc_version}")
        else()
          set(NETDATA_RPM_DOC_DIR "usr/share/doc/netdata")
        endif()
        install(FILES
                README.md
                COMPONENT netdata
                DESTINATION ${NETDATA_RPM_DOC_DIR})
endif()

if(NOT OS_WINDOWS)
  install(PROGRAMS
          packaging/installer/netdata-updater.sh
          COMPONENT netdata
          DESTINATION ${LIBEXEC_DEST})

  # user-facing support-bundle tool: on PATH as `netdata-support-bundle`, like netdatacli
  install(PROGRAMS
          packaging/installer/netdata-support-bundle
          COMPONENT netdata
          DESTINATION "${BINDIR}")

  install(FILES
          system/netdata.conf
          system/netdata-updater.conf
          COMPONENT netdata
          DESTINATION ${NETDATA_CONF_DEST})

  if(BUILD_FOR_PACKAGING AND NETDATA_PACKAGING_FORMAT STREQUAL "rpm")
    # RPMs ship the stock copies under /usr/lib/netdata/conf.d in addition to
    # the %config(noreplace) ones in /etc/netdata.
    install(FILES
            system/netdata.conf
            system/netdata-updater.conf
            COMPONENT netdata
            DESTINATION ${LIBCONFIG_DEST})
  endif()

  configure_file(system/cron/netdata-updater-daily.in
                 system/cron/netdata-updater-daily
                 @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/cron/netdata-updater-daily
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/cron)

  configure_file(system/systemd/netdata-updater.service.in
                 system/systemd/netdata-updater.service
                 @ONLY)
  install(FILES
          ${CMAKE_BINARY_DIR}/system/systemd/netdata-updater.service
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd)

  install(FILES
          system/systemd/netdata-updater.timer
          COMPONENT netdata
          DESTINATION ${SYSTEM_DEST}/systemd)

  if(NETDATA_STAGE_HOST_FILES)
    install(FILES
            ${CMAKE_BINARY_DIR}/system/systemd/netdata-updater.service
            COMPONENT netdata
            DESTINATION ${HOST_SYSTEMD_UNIT_DEST})
    install(FILES
            system/systemd/netdata-updater.timer
            COMPONENT netdata
            DESTINATION ${HOST_SYSTEMD_UNIT_DEST})
  endif()
endif()

#
# TODO: check the following files for correct substitutions
#
configure_file(src/daemon/anonymous-statistics.sh.in src/daemon/anonymous-statistics.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/daemon/anonymous-statistics.sh
        COMPONENT netdata
        DESTINATION ${PLUGINS_DEST})

configure_file(src/daemon/get-kubernetes-labels.sh.in src/daemon/get-kubernetes-labels.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/daemon/get-kubernetes-labels.sh
        COMPONENT netdata
        DESTINATION ${PLUGINS_DEST})

install(PROGRAMS
        src/daemon/system-info.sh
        COMPONENT netdata
        DESTINATION ${PLUGINS_DEST})

#
# health files
#

file(GLOB_RECURSE HEALTH_CONF_FILES "src/health/health.d/*.conf")
install(FILES
        ${HEALTH_CONF_FILES}
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST}/health.d)

configure_file(src/health/notifications/alarm-notify.sh.in src/health/notifications/alarm-notify.sh @ONLY)
install(PROGRAMS
        ${CMAKE_BINARY_DIR}/src/health/notifications/alarm-notify.sh
        COMPONENT netdata
        DESTINATION ${PLUGINS_DEST})

install(FILES
        src/health/notifications/health_alarm_notify.conf
        src/health/notifications/health_email_recipients.conf
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST})
#
# charts.d plugin
#

if(ENABLE_PLUGIN_CHARTS)
  install(DIRECTORY COMPONENT plugin-chartsd DESTINATION ${CONFIG_DEST}/charts.d)

  configure_file(src/collectors/charts.d.plugin/charts.d.plugin.in src/collectors/charts.d.plugin/charts.d.plugin @ONLY)
  install(PROGRAMS
          ${CMAKE_BINARY_DIR}/src/collectors/charts.d.plugin/charts.d.plugin
          COMPONENT plugin-chartsd
          DESTINATION ${PLUGINS_DEST})

  install(PROGRAMS
          src/collectors/charts.d.plugin/charts.d.dryrun-helper.sh
          COMPONENT plugin-chartsd
          DESTINATION ${PLUGINS_DEST})

  install(FILES
          src/collectors/charts.d.plugin/charts.d.conf
          COMPONENT plugin-chartsd
          DESTINATION ${LIBCONFIG_DEST})

  install(PROGRAMS
          src/collectors/charts.d.plugin/example/example.chart.sh
          src/collectors/charts.d.plugin/libreswan/libreswan.chart.sh
          src/collectors/charts.d.plugin/opensips/opensips.chart.sh
          COMPONENT plugin-chartsd
          DESTINATION ${LIBEXEC_DEST}/charts.d)

  install(FILES
          src/collectors/charts.d.plugin/example/example.conf
          src/collectors/charts.d.plugin/libreswan/libreswan.conf
          src/collectors/charts.d.plugin/opensips/opensips.conf
          COMPONENT plugin-chartsd
          DESTINATION ${LIBCONFIG_DEST}/charts.d)

  netdata_add_deb_copyright(plugin-chartsd netdata-plugin-chartsd)
endif()

# This is needed both by the TC plugin (which only gets built on Linux) and the charts plugin.
if(OS_LINUX OR ENABLE_PLUGIN_CHARTS)
  install(FILES
          src/collectors/charts.d.plugin/loopsleepms.sh.inc
          COMPONENT netdata
          DESTINATION ${PLUGINS_DEST})
endif()

#
# tc-qos-helper
#

if(OS_LINUX)
  configure_file(src/collectors/tc.plugin/tc-qos-helper.sh.in src/collectors/tc.plugin/tc-qos-helper.sh @ONLY)
  install(PROGRAMS
          ${CMAKE_BINARY_DIR}/src/collectors/tc.plugin/tc-qos-helper.sh
          COMPONENT netdata
          DESTINATION ${PLUGINS_DEST})
endif()

# confs
install(FILES
        src/collectors/systemd-journal.plugin/schema.d/systemd-journal%3Amonitored-directories.json
        src/health/schema.d/health%3Aalert%3Aprototype.json
        COMPONENT netdata
        DESTINATION ${LIBCONFIG_DEST}/schema.d)

#
# python.d plugin
#

if(ENABLE_PLUGIN_PYTHON)
  install(DIRECTORY COMPONENT plugin-pythond DESTINATION ${CONFIG_DEST}/python.d)

  configure_file(src/collectors/python.d.plugin/python.d.plugin.in src/collectors/python.d.plugin/python.d.plugin @ONLY)
  install(PROGRAMS ${CMAKE_BINARY_DIR}/src/collectors/python.d.plugin/python.d.plugin
          COMPONENT plugin-pythond
          DESTINATION ${PLUGINS_DEST})

  install(DIRECTORY src/collectors/python.d.plugin/python_modules
          COMPONENT plugin-pythond
          DESTINATION ${LIBEXEC_DEST}/python.d)

  if(OS_WINDOWS)
    include(NetdataUtil)
    precompile_python(${LIBEXEC_DEST}/python.d plugin-pythond)
  endif()

  install(FILES src/collectors/python.d.plugin/python.d.conf
          COMPONENT plugin-pythond
          DESTINATION ${LIBCONFIG_DEST})

  install(FILES
          src/collectors/python.d.plugin/am2320/am2320.conf
          src/collectors/python.d.plugin/go_expvar/go_expvar.conf
          src/collectors/python.d.plugin/haproxy/haproxy.conf
          src/collectors/python.d.plugin/pandas/pandas.conf
          src/collectors/python.d.plugin/traefik/traefik.conf
          COMPONENT plugin-pythond
          DESTINATION ${LIBCONFIG_DEST}/python.d)

  install(FILES
          src/collectors/python.d.plugin/am2320/am2320.chart.py
          src/collectors/python.d.plugin/go_expvar/go_expvar.chart.py
          src/collectors/python.d.plugin/haproxy/haproxy.chart.py
          src/collectors/python.d.plugin/pandas/pandas.chart.py
          src/collectors/python.d.plugin/traefik/traefik.chart.py
          COMPONENT plugin-pythond
          DESTINATION ${LIBEXEC_DEST}/python.d)

  netdata_add_deb_copyright(plugin-pythond netdata-plugin-pythond)
endif()

#
# ioping.plugin
#

configure_file(src/collectors/ioping.plugin/ioping.plugin.in src/collectors/ioping.plugin/ioping.plugin @ONLY)
install(PROGRAMS ${CMAKE_BINARY_DIR}/src/collectors/ioping.plugin/ioping.plugin
        COMPONENT netdata
        DESTINATION ${PLUGINS_DEST})

#
# go.d.plugin
#
if(ENABLE_PLUGIN_GO)
    install(DIRECTORY COMPONENT plugin-go DESTINATION ${CONFIG_DEST}/go.d)

    install(FILES src/go/plugin/go.d/config/go.d.conf
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST})

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d)
    file(GLOB GO_CONF_FILES src/go/plugin/go.d/config/go.d/*.conf)
    install(FILES ${GO_CONF_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/sd)
    file(GLOB GO_SD_CONF_FILES src/go/plugin/go.d/config/go.d/sd/*.conf)
    install(FILES ${GO_SD_CONF_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/sd)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/ss)
    file(GLOB GO_SS_CONF_FILES src/go/plugin/go.d/config/go.d/ss/*.conf)
    install(FILES ${GO_SS_CONF_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/ss)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.profiles)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.profiles/default)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.profiles/metadata)

    file(GLOB GO_SNMP_PROFILE_FILES src/go/plugin/go.d/config/go.d/snmp.profiles/default/*.yaml)
    install(FILES ${GO_SNMP_PROFILE_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.profiles/default)
    file(GLOB GO_SNMP_META_FILES src/go/plugin/go.d/config/go.d/snmp.profiles/metadata/*.yaml)
    install(FILES ${GO_SNMP_META_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.profiles/metadata)
    install(FILES src/go/plugin/go.d/config/go.d/snmp.profiles/metadata/iana-enterprise-numbers.txt
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.profiles/metadata)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.trap-profiles)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.trap-profiles/default)

    file(GLOB GO_SNMP_TRAP_PROFILE_FILES CONFIGURE_DEPENDS
            src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/*.yaml
            src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default/*.yml)

    set(SNMP_TRAP_PROFILE_PACK_DIR "${CMAKE_BINARY_DIR}/snmp-trap-profile-pack")
    set(SNMP_TRAP_PROFILE_PACK_DEFAULT_DIR "${SNMP_TRAP_PROFILE_PACK_DIR}/default")
    set(SNMP_TRAP_PROFILE_PACK_STAMP "${SNMP_TRAP_PROFILE_PACK_DIR}/.compressed-stamp")
    set(SNMP_TRAP_PROFILE_PACK_CLEAN_SCRIPT "${CMAKE_BINARY_DIR}/snmp-trap-profile-pack-clean.cmake")
    file(WRITE "${SNMP_TRAP_PROFILE_PACK_CLEAN_SCRIPT}"
        "file(REMOVE_RECURSE [[${SNMP_TRAP_PROFILE_PACK_DIR}]])\n")
    file(MAKE_DIRECTORY "${SNMP_TRAP_PROFILE_PACK_DEFAULT_DIR}")

    add_custom_command(
            OUTPUT "${SNMP_TRAP_PROFILE_PACK_STAMP}"
            COMMAND "${CMAKE_COMMAND}" -P "${SNMP_TRAP_PROFILE_PACK_CLEAN_SCRIPT}"
            COMMAND "${CMAKE_COMMAND}" -E make_directory "${SNMP_TRAP_PROFILE_PACK_DEFAULT_DIR}"
            COMMAND "${CMAKE_COMMAND}" -E copy_directory
                "${CMAKE_SOURCE_DIR}/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/default"
                "${SNMP_TRAP_PROFILE_PACK_DEFAULT_DIR}"
            COMMAND "${CMAKE_COMMAND}" -E copy_if_different
                "${CMAKE_SOURCE_DIR}/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json"
                "${SNMP_TRAP_PROFILE_PACK_DIR}/catalogue.json"
            COMMAND "${CMAKE_COMMAND}" -E env
                GOROOT=${GO_ROOT}
                CGO_ENABLED=0
                GOPROXY=https://proxy.golang.org,direct
                "${GO_EXECUTABLE}" run -buildvcs=false -ldflags "${GO_LDFLAGS}" ./cmd/snmptrapprofilegen compress-zstd --rm
                "${SNMP_TRAP_PROFILE_PACK_DEFAULT_DIR}"
                "${SNMP_TRAP_PROFILE_PACK_DIR}/catalogue.json"
            COMMAND "${CMAKE_COMMAND}" -E touch "${SNMP_TRAP_PROFILE_PACK_STAMP}"
            DEPENDS
                ${GO_SNMP_TRAP_PROFILE_FILES}
                "${CMAKE_SOURCE_DIR}/src/go/plugin/go.d/config/go.d/snmp.trap-profiles/catalogue.json"
                ${snmp_trap_profile_gen_DEPS}
            COMMENT "Compressing SNMP trap profile pack"
            WORKING_DIRECTORY "${CMAKE_SOURCE_DIR}/src/go"
            VERBATIM)
    add_custom_target(snmp_trap_profile_pack ALL
            DEPENDS "${SNMP_TRAP_PROFILE_PACK_STAMP}")

    install(CODE "
        if(NOT EXISTS \"${SNMP_TRAP_PROFILE_PACK_STAMP}\")
            message(FATAL_ERROR \"SNMP trap profile pack is not built; build target snmp_trap_profile_pack before install\")
        endif()
    " COMPONENT plugin-go)
    install(DIRECTORY "${SNMP_TRAP_PROFILE_PACK_DEFAULT_DIR}/"
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.trap-profiles/default
            FILES_MATCHING
                PATTERN "*.yaml.zst"
                PATTERN "*.yml.zst")
    install(FILES "${SNMP_TRAP_PROFILE_PACK_DIR}/catalogue.json.zst"
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.trap-profiles)
    install(FILES src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/snmp.trap-profiles)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/azure_monitor.profiles)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/azure_monitor.profiles/default)
    file(GLOB GO_AZURE_MON_PROFILE_FILES src/go/plugin/go.d/config/go.d/azure_monitor.profiles/default/*.yaml)
    install(FILES ${GO_AZURE_MON_PROFILE_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/azure_monitor.profiles/default)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/prometheus.profiles)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/prometheus.profiles/default)
    file(GLOB GO_PROMETHEUS_PROFILE_FILES src/go/plugin/go.d/config/go.d/prometheus.profiles/default/*.yaml)
    install(FILES ${GO_PROMETHEUS_PROFILE_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/prometheus.profiles/default)

    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/cloudwatch.profiles)
    install(DIRECTORY
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/cloudwatch.profiles/default)
    file(GLOB GO_CLOUDWATCH_PROFILE_FILES src/go/plugin/go.d/config/go.d/cloudwatch.profiles/default/*.yaml)
    install(FILES ${GO_CLOUDWATCH_PROFILE_FILES}
            COMPONENT plugin-go
            DESTINATION ${LIBCONFIG_DEST}/go.d/cloudwatch.profiles/default)

    netdata_add_deb_copyright(plugin-go netdata-plugin-go)
endif()

#
# dashboard
#

if(ENABLE_DASHBOARD)
  include(NetdataDashboard)
  bundle_dashboard()

  netdata_add_deb_copyright(dashboard netdata-dashboard)
endif()

#
# Ship CMakeCache.txt as an archive
#

install(CODE "
        execute_process(COMMAND gzip -nc \"${CMAKE_BINARY_DIR}/CMakeCache.txt\"
                        OUTPUT_FILE \"${BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME}\"
                        WORKING_DIRECTORY \"${CMAKE_BINARY_DIR}\"
                        RESULT_VARIABLE result)
        if(NOT result EQUAL 0)
                message(WARNING \"Failed to compress CMakeCache.txt\")
        endif()

        file(INSTALL \"${CMAKE_BINARY_DIR}/${BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME}\"
             DESTINATION \"\${CMAKE_INSTALL_PREFIX}/${BUILD_INFO_CMAKE_CACHE_ARCHIVE_PATH}\")
")

#
# vendor msys stuff on Windows
#

if(OS_WINDOWS)
        install(FILES /usr/bin/msys-protobuf-32.dll
                      /usr/bin/msys-yaml-0-2.dll
                      /usr/bin/msys-uv-1.dll
                      DESTINATION "${BINDIR}")

        # user-facing support-bundle tool (Windows PowerShell implementation)
        install(PROGRAMS packaging/installer/netdata-support-bundle.ps1
                COMPONENT netdata
                DESTINATION ${LIBEXEC_DEST})

        # Make bash & netdata happy
        install(DIRECTORY DESTINATION tmp)

        # Make curl work with ssl
        install(DIRECTORY /usr/ssl DESTINATION usr)
endif()

#
# Optional: render integration docs from metadata.yaml
#

include(NetdataRenderDocs)

# DEB policy: every binary package carries a copyright file. The plugins add
# theirs beside their install rules; these two components are defined here.
netdata_add_deb_copyright(netdata netdata)
netdata_add_deb_copyright(user netdata-user)

#
# Include packaging logic
#

include(Packaging)

#
# Optional convenience: wire the netdata-build MCP server (packaging/tools/
# automation/mcp) into a local agent client. Explicit one-off — `ninja setup-mcp`
# — not part of the build (no ALL). Mutates the USER's global opencode/Claude
# config for this checkout; never touches the repo. See the tool's README.
#
add_custom_target(setup-mcp
        COMMAND python3
                "${CMAKE_SOURCE_DIR}/packaging/tools/automation/mcp/scripts/setup_mcp.py"
                --tool all --source-dir "${CMAKE_SOURCE_DIR}"
        USES_TERMINAL
        COMMENT "Configuring netdata-build MCP server for opencode/Claude Code (global config)")
