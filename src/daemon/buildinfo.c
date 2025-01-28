// SPDX-License-Identifier: GPL-3.0-or-later

#define RRDHOST_SYSTEM_INFO_INTERNALS
#include <stdio.h>
#include "./config.h"
#include "common.h"
#include "buildinfo.h"

typedef enum __attribute__((packed)) {
    BIB_PACKAGING_NETDATA_VERSION,
    BIB_PACKAGING_INSTALL_TYPE,
    BIB_PACKAGING_ARCHITECTURE,
    BIB_PACKAGING_DISTRO,
    BIB_PACKAGING_CONFIGURE_OPTIONS,
    BIB_DIR_USER_CONFIG,
    BIB_DIR_STOCK_CONFIG,
    BIB_DIR_CACHE,
    BIB_DIR_LIB,
    BIB_DIR_PLUGINS,
    BIB_DIR_WEB,
    BIB_DIR_LOG,
    BIB_DIR_LOCK,
    BIB_DIR_HOME,
    BIB_OS_KERNEL_NAME,
    BIB_OS_KERNEL_VERSION,
    BIB_OS_NAME,
    BIB_OS_ID,
    BIB_OS_ID_LIKE,
    BIB_OS_VERSION,
    BIB_OS_VERSION_ID,
    BIB_OS_DETECTION,
    BIB_HW_CPU_CORES,
    BIB_HW_CPU_FREQUENCY,
    BIB_HW_RAM_SIZE,
    BIB_HW_DISK_SPACE,
    BIB_HW_ARCHITECTURE,
    BIB_HW_VIRTUALIZATION,
    BIB_HW_VIRTUALIZATION_DETECTION,
    BIB_CONTAINER_NAME,
    BIB_CONTAINER_DETECTION,
    BIB_CONTAINER_ORCHESTRATOR,
    BIB_CONTAINER_OS_NAME,
    BIB_CONTAINER_OS_ID,
    BIB_CONTAINER_OS_ID_LIKE,
    BIB_CONTAINER_OS_VERSION,
    BIB_CONTAINER_OS_VERSION_ID,
    BIB_CONTAINER_OS_DETECTION,
    BIB_FEATURE_BUILT_FOR,
    BIB_FEATURE_CLOUD,
    BIB_FEATURE_HEALTH,
    BIB_FEATURE_STREAMING,
    BIB_FEATURE_BACKFILLING,
    BIB_FEATURE_REPLICATION,
    BIB_FEATURE_STREAMING_COMPRESSION,
    BIB_FEATURE_CONTEXTS,
    BIB_FEATURE_TIERING,
    BIB_FEATURE_ML,
    BIB_FEATURE_ALLOCATOR,
    BIB_DB_DBENGINE,
    BIB_DB_ALLOC,
    BIB_DB_RAM,
    BIB_DB_NONE,
    BIB_CONNECTIVITY_ACLK,
    BIB_CONNECTIVITY_HTTPD_STATIC,
    BIB_CONNECTIVITY_HTTPD_H2O,
    BIB_CONNECTIVITY_WEBRTC,
    BIB_CONNECTIVITY_NATIVE_HTTPS,
    BIB_CONNECTIVITY_TLS_HOST_VERIFY,
    BIB_LIB_LZ4,
    BIB_LIB_ZSTD,
    BIB_LIB_ZLIB,
    BIB_LIB_BROTLI,
    BIB_LIB_PROTOBUF,
    BIB_LIB_OPENSSL,
    BIB_LIB_LIBDATACHANNEL,
    BIB_LIB_JSONC,
    BIB_LIB_LIBCAP,
    BIB_LIB_LIBCRYPTO,
    BIB_LIB_LIBYAML,
    BIB_LIB_LIBMNL,
    BIB_PLUGIN_APPS,
    BIB_PLUGIN_LINUX_CGROUPS,
    BIB_PLUGIN_LINUX_CGROUP_NETWORK,
    BIB_PLUGIN_LINUX_PROC,
    BIB_PLUGIN_LINUX_TC,
    BIB_PLUGIN_LINUX_DISKSPACE,
    BIB_PLUGIN_FREEBSD,
    BIB_PLUGIN_MACOS,
    BIB_PLUGIN_WINDOWS,
    BIB_PLUGIN_STATSD,
    BIB_PLUGIN_TIMEX,
    BIB_PLUGIN_IDLEJITTER,
    BIB_PLUGIN_BASH,
    BIB_PLUGIN_DEBUGFS,
    BIB_PLUGIN_CUPS,
    BIB_PLUGIN_EBPF,
    BIB_PLUGIN_FREEIPMI,
    BIB_PLUGIN_NETWORK_VIEWER,
    BIB_PLUGIN_SYSTEMD_JOURNAL,
    BIB_PLUGIN_WINDOWS_EVENTS,
    BIB_PLUGIN_NFACCT,
    BIB_PLUGIN_PERF,
    BIB_PLUGIN_SLABINFO,
    BIB_PLUGIN_XEN,
    BIB_PLUGIN_XEN_VBD_ERROR,
    BIB_EXPORT_AWS_KINESIS,
    BIB_EXPORT_GCP_PUBSUB,
    BIB_EXPORT_MONGOC,
    BIB_EXPORT_PROMETHEUS_EXPORTER,
    BIB_EXPORT_PROMETHEUS_REMOTE_WRITE,
    BIB_EXPORT_GRAPHITE,
    BIB_EXPORT_GRAPHITE_HTTP,
    BIB_EXPORT_JSON,
    BIB_EXPORT_JSON_HTTP,
    BIB_EXPORT_OPENTSDB,
    BIB_EXPORT_OPENTSDB_HTTP,
    BIB_EXPORT_ALLMETRICS,
    BIB_EXPORT_SHELL,
    BIB_DEVEL_TRACE_ALLOCATIONS,
    BIB_DEVELOPER_MODE,
    BIB_RUNTIME_PROFILE,
    BIB_RUNTIME_PARENT,
    BIB_RUNTIME_CHILD,
    BIB_RUNTIME_MEM_TOTAL,
    BIB_RUNTIME_MEM_AVAIL,

    // leave this last
    BIB_TERMINATOR,
} BUILD_INFO_SLOT;

typedef enum __attribute__((packed)) {
    BIC_PACKAGING,
    BIC_DIRECTORIES,
    BIC_OPERATING_SYSTEM,
    BIC_HARDWARE,
    BIC_CONTAINER,
    BIC_FEATURE,
    BIC_DATABASE,
    BIC_CONNECTIVITY,
    BIC_LIBS,
    BIC_PLUGINS,
    BIC_EXPORTERS,
    BIC_DEBUG_DEVEL,
    BIC_RUNTIME,
} BUILD_INFO_CATEGORY;

typedef enum __attribute__((packed)) {
    BIT_BOOLEAN,
    BIT_STRING,
} BUILD_INFO_TYPE;

static struct {
    BUILD_INFO_CATEGORY category;
    BUILD_INFO_TYPE type;
    const char *analytics;
    const char *print;
    const char *json;
    bool status;
    const char *value;
} BUILD_INFO[] = {
        [BIB_PACKAGING_NETDATA_VERSION] = {
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Netdata Version",
                .json = "version",
                .value = "unknown",
        },
        [BIB_PACKAGING_INSTALL_TYPE] = {
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Installation Type",
                .json = "type",
                .value = "unknown",
        },
        [BIB_PACKAGING_ARCHITECTURE] = {
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Package Architecture",
                .json = "arch",
                .value = "unknown",
        },
        [BIB_PACKAGING_DISTRO] = {
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Package Distro",
                .json = "distro",
                .value = "unknown",
        },
        [BIB_PACKAGING_CONFIGURE_OPTIONS] = {
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Configure Options",
                .json = "configure",
                .value = "unknown",
        },
        [BIB_DIR_USER_CONFIG] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "User Configurations",
                .json = "user_config",
                .value = CONFIG_DIR,
        },
        [BIB_DIR_STOCK_CONFIG] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Stock Configurations",
                .json = "stock_config",
                .value = LIBCONFIG_DIR,
        },
        [BIB_DIR_CACHE] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Ephemeral Databases (metrics data, metadata)",
                .json = "ephemeral_db",
                .value = CACHE_DIR,
        },
        [BIB_DIR_LIB] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Permanent Databases",
                .json = "permanent_db",
                .value = VARLIB_DIR,
        },
        [BIB_DIR_PLUGINS] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Plugins",
                .json = "plugins",
                .value = PLUGINS_DIR,
        },
        [BIB_DIR_WEB] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Static Web Files",
                .json = "web",
                .value = WEB_DIR,
        },
        [BIB_DIR_LOG] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Log Files",
                .json = "logs",
                .value = LOG_DIR,
        },
        [BIB_DIR_LOCK] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Lock Files",
                .json = "locks",
                .value = VARLIB_DIR "/lock",
        },
        [BIB_DIR_HOME] = {
                .category = BIC_DIRECTORIES,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Home",
                .json = "home",
                .value = VARLIB_DIR,
        },
        [BIB_OS_KERNEL_NAME] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Kernel",
                .json = "kernel",
                .value = "unknown",
        },
        [BIB_OS_KERNEL_VERSION] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Kernel Version",
                .json = "kernel_version",
                .value = "unknown",
        },
        [BIB_OS_NAME] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System",
                .json = "os",
                .value = "unknown",
        },
        [BIB_OS_ID] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System ID",
                .json = "id",
                .value = "unknown",
        },
        [BIB_OS_ID_LIKE] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System ID Like",
                .json = "id_like",
                .value = "unknown",
        },
        [BIB_OS_VERSION] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System Version",
                .json = "version",
                .value = "unknown",
        },
        [BIB_OS_VERSION_ID] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System Version ID",
                .json = "version_id",
                .value = "unknown",
        },
        [BIB_OS_DETECTION] = {
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Detection",
                .json = "detection",
                .value = "unknown",
        },
        [BIB_HW_CPU_CORES] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "CPU Cores",
                .json = "cpu_cores",
                .value = "unknown",
        },
        [BIB_HW_CPU_FREQUENCY] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "CPU Frequency",
                .json = "cpu_frequency",
                .value = "unknown",
        },
        [BIB_HW_ARCHITECTURE] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "CPU Architecture",
                .json = "cpu_architecture",
                .value = "unknown",
        },
        [BIB_HW_RAM_SIZE] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "RAM Bytes",
                .json = "ram",
                .value = "unknown",
        },
        [BIB_HW_DISK_SPACE] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Disk Capacity",
                .json = "disk",
                .value = "unknown",
        },
        [BIB_HW_VIRTUALIZATION] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Virtualization Technology",
                .json = "virtualization",
                .value = "unknown",
        },
        [BIB_HW_VIRTUALIZATION_DETECTION] = {
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Virtualization Detection",
                .json = "virtualization_detection",
                .value = "unknown",
        },
        [BIB_CONTAINER_NAME] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container",
                .json = "container",
                .value = "unknown",
        },
        [BIB_CONTAINER_DETECTION] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Detection",
                .json = "container_detection",
                .value = "unknown",
        },
        [BIB_CONTAINER_ORCHESTRATOR] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Orchestrator",
                .json = "orchestrator",
                .value = "unknown",
        },
        [BIB_CONTAINER_OS_NAME] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System",
                .json = "os",
                .value = "unknown",
        },
        [BIB_CONTAINER_OS_ID] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System ID",
                .json = "os_id",
                .value = "unknown",
        },
        [BIB_CONTAINER_OS_ID_LIKE] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System ID Like",
                .json = "os_id_like",
                .value = "unknown",
        },
        [BIB_CONTAINER_OS_VERSION] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System Version",
                .json = "version",
                .value = "unknown",
        },
        [BIB_CONTAINER_OS_VERSION_ID] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System Version ID",
                .json = "version_id",
                .value = "unknown",
        },
        [BIB_CONTAINER_OS_DETECTION] = {
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System Detection",
                .json = "detection",
                .value = "unknown",
        },
        [BIB_FEATURE_BUILT_FOR] = {
                .category = BIC_FEATURE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Built For",
                .json = "built-for",
                .value = "unknown",
        },
        [BIB_FEATURE_CLOUD] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = "Netdata Cloud",
                .print = "Netdata Cloud",
                .json = "cloud",
                .value = NULL,
        },
        [BIB_FEATURE_HEALTH] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Health (trigger alerts and send notifications)",
                .json = "health",
                .value = NULL,
        },
        [BIB_FEATURE_STREAMING] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Streaming (stream metrics to parent Netdata servers)",
                .json = "streaming",
                .value = NULL,
        },
        [BIB_FEATURE_BACKFILLING] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Back-filling (of higher database tiers)",
                .json = "back-filling",
                .value = NULL,
        },
        [BIB_FEATURE_REPLICATION] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Replication (fill the gaps of parent Netdata servers)",
                .json = "replication",
                .value = NULL,
        },
        [BIB_FEATURE_STREAMING_COMPRESSION] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = "Stream Compression",
                .print = "Streaming and Replication Compression",
                .json = "stream-compression",
                .value = NULL,
        },
        [BIB_FEATURE_CONTEXTS] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Contexts (index all active and archived metrics)",
                .json = "contexts",
                .value = NULL,
        },
        [BIB_FEATURE_TIERING] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Tiering (multiple dbs with different metrics resolution)",
                .json = "tiering",
                .value = TOSTRING(RRD_STORAGE_TIERS),
        },
        [BIB_FEATURE_ML] = {
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = "Machine Learning",
                .print = "Machine Learning",
                .json = "ml",
                .value = NULL,
        },
        [BIB_FEATURE_ALLOCATOR] = {
                .category = BIC_FEATURE,
                .type = BIT_STRING,
                .analytics = "allocator",
                .print = "Memory Allocator",
                .json = "allocator",
                .value = NULL,
        },
        [BIB_DB_DBENGINE] = {
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = "dbengine",
                .print = "dbengine (compression)",
                .json = "dbengine",
                .value = NULL,
        },
        [BIB_DB_ALLOC] = {
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "alloc",
                .json = "alloc",
                .value = NULL,
        },
        [BIB_DB_RAM] = {
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "ram",
                .json = "ram",
                .value = NULL,
        },
        [BIB_DB_NONE] = {
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "none",
                .json = "none",
                .value = NULL,
        },
        [BIB_CONNECTIVITY_ACLK] = {
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "ACLK (Agent-Cloud Link: MQTT over WebSockets over TLS)",
                .json = "aclk",
                .value = NULL,
        },
        [BIB_CONNECTIVITY_HTTPD_STATIC] = {
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "static (Netdata internal web server)",
                .json = "static",
                .value = NULL,
        },
        [BIB_CONNECTIVITY_HTTPD_H2O] = {
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "h2o (web server)",
                .json = "h2o",
                .value = NULL,
        },
        [BIB_CONNECTIVITY_WEBRTC] = {
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "WebRTC (experimental)",
                .json = "webrtc",
                .value = NULL,
        },
        [BIB_CONNECTIVITY_NATIVE_HTTPS] = {
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = "Native HTTPS",
                .print = "Native HTTPS (TLS Support)",
                .json = "native-https",
                .value = NULL,
        },
        [BIB_CONNECTIVITY_TLS_HOST_VERIFY] = {
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = "TLS Host Verification",
                .print = "TLS Host Verification",
                .json = "tls-host-verify",
                .value = NULL,
        },
        [BIB_LIB_LZ4] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "LZ4 (extremely fast lossless compression algorithm)",
                .json = "lz4",
                .value = NULL,
        },
        [BIB_LIB_ZSTD] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "ZSTD (fast, lossless compression algorithm)",
                .json = "zstd",
                .value = NULL,
        },
        [BIB_LIB_ZLIB] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "zlib",
                .print = "zlib (lossless data-compression library)",
                .json = "zlib",
                .value = NULL,
        },
        [BIB_LIB_BROTLI] = {
            .category = BIC_LIBS,
            .type = BIT_BOOLEAN,
            .analytics = NULL,
            .print = "Brotli (generic-purpose lossless compression algorithm)",
            .json = "brotli",
            .value = NULL,
        },
        [BIB_LIB_PROTOBUF] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "protobuf",
                .print = "protobuf (platform-neutral data serialization protocol)",
                .json = "protobuf",
                .value = NULL,
        },
        [BIB_LIB_OPENSSL] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "OpenSSL (cryptography)",
                .json = "openssl",
                .value = NULL,
        },
        [BIB_LIB_LIBDATACHANNEL] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "libdatachannel (stand-alone WebRTC data channels)",
                .json = "libdatachannel",
                .value = NULL,
        },
        [BIB_LIB_JSONC] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "JSON-C",
                .print = "JSON-C (lightweight JSON manipulation)",
                .json = "jsonc",
                .value = NULL,
        },
        [BIB_LIB_LIBCAP] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "libcap",
                .print = "libcap (Linux capabilities system operations)",
                .json = "libcap",
                .value = NULL,
        },
        [BIB_LIB_LIBCRYPTO] = {
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "libcrypto",
                .print = "libcrypto (cryptographic functions)",
                .json = "libcrypto",
                .value = NULL,
        },
        [BIB_LIB_LIBYAML] = {
            .category = BIC_LIBS,
            .type = BIT_BOOLEAN,
            .analytics = "libyaml",
            .print = "libyaml (library for parsing and emitting YAML)",
            .json = "libyaml",
            .value = NULL,
        },
        [BIB_LIB_LIBMNL] = {
            .category = BIC_LIBS,
            .type = BIT_BOOLEAN,
            .analytics = "libmnl",
            .print = "libmnl (library for working with netfilter)",
            .json = "libmnl",
            .value = NULL,
        },
        [BIB_PLUGIN_APPS] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "apps",
                .print = "apps (monitor processes)",
                .json = "apps",
                .value = NULL,
        },
        [BIB_PLUGIN_LINUX_CGROUPS] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "cgroups (monitor containers and VMs)",
                .json = "cgroups",
                .value = NULL,
        },
        [BIB_PLUGIN_LINUX_CGROUP_NETWORK] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "cgroup Network Tracking",
                .print = "cgroup-network (associate interfaces to CGROUPS)",
                .json = "cgroup-network",
                .value = NULL,
        },
        [BIB_PLUGIN_LINUX_PROC] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "proc (monitor Linux systems)",
                .json = "proc",
                .value = NULL,
        },
        [BIB_PLUGIN_LINUX_TC] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "tc (monitor Linux network QoS)",
                .json = "tc",
                .value = NULL,
        },
        [BIB_PLUGIN_LINUX_DISKSPACE] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "diskspace (monitor Linux mount points)",
                .json = "diskspace",
                .value = NULL,
        },
        [BIB_PLUGIN_FREEBSD] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "freebsd (monitor FreeBSD systems)",
                .json = "freebsd",
                .value = NULL,
        },
        [BIB_PLUGIN_MACOS] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "macos (monitor MacOS systems)",
                .json = "macos",
                .value = NULL,
        },
        [BIB_PLUGIN_WINDOWS] = {
            .category = BIC_PLUGINS,
            .type = BIT_BOOLEAN,
            .analytics = NULL,
            .print = "windows (monitor Windows systems)",
            .json = "windows",
            .value = NULL,
        },
        [BIB_PLUGIN_STATSD] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "statsd (collect custom application metrics)",
                .json = "statsd",
                .value = NULL,
        },
        [BIB_PLUGIN_TIMEX] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "timex (check system clock synchronization)",
                .json = "timex",
                .value = NULL,
        },
        [BIB_PLUGIN_IDLEJITTER] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "idlejitter (check system latency and jitter)",
                .json = "idlejitter",
                .value = NULL,
        },
        [BIB_PLUGIN_BASH] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "bash (support shell data collection jobs - charts.d)",
                .json = "charts.d",
                .value = NULL,
        },
        [BIB_PLUGIN_DEBUGFS] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "debugfs",
                .print = "debugfs (kernel debugging metrics)",
                .json = "debugfs",
                .value = NULL,
        },
        [BIB_PLUGIN_CUPS] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "CUPS",
                .print = "cups (monitor printers and print jobs)",
                .json = "cups",
                .value = NULL,
        },
        [BIB_PLUGIN_EBPF] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "EBPF",
                .print = "ebpf (monitor system calls)",
                .json = "ebpf",
                .value = NULL,
        },
        [BIB_PLUGIN_FREEIPMI] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "IPMI",
                .print = "freeipmi (monitor enterprise server H/W)",
                .json = "freeipmi",
                .value = NULL,
        },
        [BIB_PLUGIN_NETWORK_VIEWER] = {
            .category = BIC_PLUGINS,
            .type = BIT_BOOLEAN,
            .analytics = "NETWORK-VIEWER",
            .print = "network-viewer (monitor TCP/UDP IPv4/6 sockets)",
            .json = "network-viewer",
            .value = NULL,
        },
        [BIB_PLUGIN_SYSTEMD_JOURNAL] = {
            .category = BIC_PLUGINS,
            .type = BIT_BOOLEAN,
            .analytics = "SYSTEMD-JOURNAL",
            .print = "systemd-journal (monitor journal logs)",
            .json = "systemd-journal",
            .value = NULL,
        },
        [BIB_PLUGIN_WINDOWS_EVENTS] = {
            .category = BIC_PLUGINS,
            .type = BIT_BOOLEAN,
            .analytics = "WINDOWS-EVENTS",
            .print = "windows-events (monitor Windows events)",
            .json = "windows-events",
            .value = NULL,
        },
        [BIB_PLUGIN_NFACCT] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "NFACCT",
                .print = "nfacct (gather netfilter accounting)",
                .json = "nfacct",
                .value = NULL,
        },
        [BIB_PLUGIN_PERF] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "perf",
                .print = "perf (collect kernel performance events)",
                .json = "perf",
                .value = NULL,
        },
        [BIB_PLUGIN_SLABINFO] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "slabinfo",
                .print = "slabinfo (monitor kernel object caching)",
                .json = "slabinfo",
                .value = NULL,
        },
        [BIB_PLUGIN_XEN] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "Xen",
                .print = "Xen",
                .json = "xen",
                .value = NULL,
        },
        [BIB_PLUGIN_XEN_VBD_ERROR] = {
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "Xen VBD Error Tracking",
                .print = "Xen VBD Error Tracking",
                .json = "xen-vbd-error",
                .value = NULL,
        },
        [BIB_EXPORT_MONGOC] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "MongoDB",
                .print = "MongoDB",
                .json = "mongodb",
                .value = NULL,
        },
        [BIB_EXPORT_GRAPHITE] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Graphite",
                .json = "graphite",
                .value = NULL,
        },
        [BIB_EXPORT_GRAPHITE_HTTP] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Graphite HTTP / HTTPS",
                .json = "graphite:http",
                .value = NULL,
        },
        [BIB_EXPORT_JSON] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "JSON",
                .json = "json",
                .value = NULL,
        },
        [BIB_EXPORT_JSON_HTTP] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "JSON HTTP / HTTPS",
                .json = "json:http",
                .value = NULL,
        },
        [BIB_EXPORT_OPENTSDB] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "OpenTSDB",
                .json = "opentsdb",
                .value = NULL,
        },
        [BIB_EXPORT_OPENTSDB_HTTP] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "OpenTSDB HTTP / HTTPS",
                .json = "opentsdb:http",
                .value = NULL,
        },
        [BIB_EXPORT_ALLMETRICS] = {
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .type = BIT_BOOLEAN,
                .print = "All Metrics API",
                .json = "allmetrics",
                .value = NULL,
        },
        [BIB_EXPORT_SHELL] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Shell (use metrics in shell scripts)",
                .json = "shell",
                .value = NULL,
        },
        [BIB_EXPORT_PROMETHEUS_EXPORTER] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Prometheus (OpenMetrics) Exporter",
                .json = "openmetrics",
                .value = NULL,
        },
        [BIB_EXPORT_PROMETHEUS_REMOTE_WRITE] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "Prometheus Remote Write",
                .print = "Prometheus Remote Write",
                .json = "prom-remote-write",
                .value = NULL,
        },
        [BIB_EXPORT_AWS_KINESIS] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "AWS Kinesis",
                .print = "AWS Kinesis",
                .json = "kinesis",
                .value = NULL,
        },
        [BIB_EXPORT_GCP_PUBSUB] = {
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "GCP PubSub",
                .print = "GCP PubSub",
                .json = "pubsub",
                .value = NULL,
        },
        [BIB_DEVEL_TRACE_ALLOCATIONS] = {
                .category = BIC_DEBUG_DEVEL,
                .type = BIT_BOOLEAN,
                .analytics = "DebugTraceAlloc",
                .print = "Trace All Netdata Allocations (with charts)",
                .json = "trace-allocations",
                .value = NULL,
        },
        [BIB_DEVELOPER_MODE] = {
                .category = BIC_DEBUG_DEVEL,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Developer Mode (more runtime checks, slower)",
                .json = "dev-mode",
                .value = NULL,
        },
        [BIB_RUNTIME_PROFILE] = {
            .category = BIC_RUNTIME,
            .type = BIT_STRING,
            .analytics = "ConfigProfile",
            .print = "Profile",
            .json = "profile",
            .value = NULL,
        },
        [BIB_RUNTIME_PARENT] = {
            .category = BIC_RUNTIME,
            .type = BIT_BOOLEAN,
            .analytics = "StreamParent",
            .print = "Stream Parent (accept data from Children)",
            .json = "parent",
            .value = NULL,
        },
        [BIB_RUNTIME_CHILD] = {
            .category = BIC_RUNTIME,
            .type = BIT_BOOLEAN,
            .analytics = "StreamChild",
            .print = "Stream Child (send data to a Parent)",
            .json = "child",
            .value = NULL,
        },
        [BIB_RUNTIME_MEM_TOTAL] = {
            .category = BIC_RUNTIME,
            .type = BIT_STRING,
            .analytics = "TotalMemory",
            .print = "Total System Memory",
            .json = "mem-total",
            .value = NULL,
        },
        [BIB_RUNTIME_MEM_AVAIL] = {
            .category = BIC_RUNTIME,
            .type = BIT_STRING,
            .analytics = "AvailableMemory",
            .print = "Available System Memory",
            .json = "mem-available",
            .value = NULL,
        },

        // leave this last
        [BIB_TERMINATOR] = {
                .category = 0,
                .type = 0,
                .analytics = NULL,
                .print = NULL,
                .json = NULL,
                .value = NULL,
        },
};

static void build_info_set_value(BUILD_INFO_SLOT slot, const char *value) {
    BUILD_INFO[slot].value = value;
}

static void build_info_append_value(BUILD_INFO_SLOT slot, const char *value) {
    size_t size = BUILD_INFO[slot].value ? strlen(BUILD_INFO[slot].value) + 1 : 0;
    size += strlen(value);
    char buf[size + 1];

    if(BUILD_INFO[slot].value) {
        strcpy(buf, BUILD_INFO[slot].value);
        strcat(buf, " ");
        strcat(buf, value);
    }
    else
        strcpy(buf, value);

    freez((void *)BUILD_INFO[slot].value);
    BUILD_INFO[slot].value = strdupz(buf);
}

static void build_info_set_value_strdupz(BUILD_INFO_SLOT slot, const char *value) {
    if(!value) value = "";
    build_info_set_value(slot, strdupz(value));
}

static void build_info_set_status(BUILD_INFO_SLOT slot, bool status) {
    BUILD_INFO[slot].status = status;
}

__attribute__((constructor)) void initialize_build_info(void) {
    build_info_set_value(BIB_PACKAGING_NETDATA_VERSION, NETDATA_VERSION);
    build_info_set_value(BIB_PACKAGING_CONFIGURE_OPTIONS, CONFIGURE_COMMAND);

#ifdef OS_LINUX
    build_info_set_status(BIB_FEATURE_BUILT_FOR, true);
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "Linux");
    build_info_set_status(BIB_PLUGIN_LINUX_CGROUPS, true);
    build_info_set_status(BIB_PLUGIN_LINUX_PROC, true);
    build_info_set_status(BIB_PLUGIN_LINUX_DISKSPACE, true);
    build_info_set_status(BIB_PLUGIN_LINUX_TC, true);
#endif
#ifdef OS_FREEBSD
    build_info_set_status(BIB_FEATURE_BUILT_FOR, true);
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "FreeBSD");
    build_info_set_status(BIB_PLUGIN_FREEBSD, true);
#endif
#ifdef OS_MACOS
    build_info_set_status(BIB_FEATURE_BUILT_FOR, true);
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "MacOS");
    build_info_set_status(BIB_PLUGIN_MACOS, true);
#endif
#ifdef OS_WINDOWS
    build_info_set_status(BIB_PLUGIN_WINDOWS, true);
    build_info_set_status(BIB_PLUGIN_WINDOWS_EVENTS, true);
    build_info_set_status(BIB_FEATURE_BUILT_FOR, true);
#if defined(__CYGWIN__) && defined(__MSYS__)
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "Windows (MSYS)");
#elif defined(__CYGWIN__)
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "Windows (CYGWIN)");
#else
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "Windows");
#endif
#endif

    build_info_set_status(BIB_FEATURE_CLOUD, true);
    build_info_set_status(BIB_CONNECTIVITY_ACLK, true);
    build_info_set_status(BIB_FEATURE_HEALTH, true);
    build_info_set_status(BIB_FEATURE_STREAMING, true);
    build_info_set_status(BIB_FEATURE_BACKFILLING, true);
    build_info_set_status(BIB_FEATURE_REPLICATION, true);

    build_info_set_status(BIB_FEATURE_STREAMING_COMPRESSION, true);

#ifdef ENABLE_ZSTD
    build_info_append_value(BIB_FEATURE_STREAMING_COMPRESSION, "zstd");
#endif
#ifdef ENABLE_LZ4
    build_info_append_value(BIB_FEATURE_STREAMING_COMPRESSION, "lz4");
#endif
    build_info_append_value(BIB_FEATURE_STREAMING_COMPRESSION, "gzip");
#ifdef ENABLE_BROTLI
    build_info_append_value(BIB_FEATURE_STREAMING_COMPRESSION, "brotli");
#endif

    build_info_set_status(BIB_FEATURE_CONTEXTS, true);
    build_info_set_status(BIB_FEATURE_TIERING, true);

#ifdef ENABLE_ML
    build_info_set_status(BIB_FEATURE_ML, true);
#endif

#if defined(ENABLE_MIMALLOC)
    build_info_set_status(BIB_FEATURE_ALLOCATOR, true);
    build_info_set_value(BIB_FEATURE_ALLOCATOR, "mimalloc");
#else
    build_info_set_status(BIB_FEATURE_ALLOCATOR, true);
    build_info_set_value(BIB_FEATURE_ALLOCATOR, "system");
#endif


#ifdef ENABLE_DBENGINE
    build_info_set_status(BIB_DB_DBENGINE, true);
#ifdef ENABLE_ZSTD
    build_info_append_value(BIB_DB_DBENGINE, "zstd");
#endif
#ifdef ENABLE_LZ4
    build_info_append_value(BIB_DB_DBENGINE, "lz4");
#endif
#endif
    build_info_set_status(BIB_DB_ALLOC, true);
    build_info_set_status(BIB_DB_RAM, true);
    build_info_set_status(BIB_DB_NONE, true);

    build_info_set_status(BIB_CONNECTIVITY_HTTPD_STATIC, true);
#ifdef ENABLE_H2O
    build_info_set_status(BIB_CONNECTIVITY_HTTPD_H2O, true);
#endif
#ifdef ENABLE_WEBRTC
    build_info_set_status(BIB_CONNECTIVITY_WEBRTC, true);
#endif
    build_info_set_status(BIB_CONNECTIVITY_NATIVE_HTTPS, true);
#if defined(HAVE_X509_VERIFY_PARAM_set1_host) && HAVE_X509_VERIFY_PARAM_set1_host == 1
    build_info_set_status(BIB_CONNECTIVITY_TLS_HOST_VERIFY, true);
#endif

#ifdef ENABLE_LZ4
    build_info_set_status(BIB_LIB_LZ4, true);
#endif
#ifdef ENABLE_ZSTD
    build_info_set_status(BIB_LIB_ZSTD, true);
#endif
#ifdef ENABLE_BROTLI
    build_info_set_status(BIB_LIB_BROTLI, true);
#endif

    build_info_set_status(BIB_LIB_ZLIB, true);

#ifdef HAVE_DLIB
    build_info_set_status(BIB_LIB_DLIB, true);
    build_info_set_value(BIB_LIB_DLIB, "bundled");
#endif

#ifdef HAVE_PROTOBUF
    build_info_set_status(BIB_LIB_PROTOBUF, true);
#ifdef BUNDLED_PROTOBUF
    build_info_set_value(BIB_LIB_PROTOBUF, "bundled");
#else
    build_info_set_value(BIB_LIB_PROTOBUF, "system");
#endif
#endif

#ifdef HAVE_LIBDATACHANNEL
    build_info_set_status(BIB_LIB_LIBDATACHANNEL, true);
#endif
    build_info_set_status(BIB_LIB_OPENSSL, true);
#ifdef ENABLE_JSONC
    build_info_set_status(BIB_LIB_JSONC, true);
#endif
#ifdef HAVE_CAPABILITY
    build_info_set_status(BIB_LIB_LIBCAP, true);
#endif
#ifdef HAVE_CRYPTO
    build_info_set_status(BIB_LIB_LIBCRYPTO, true);
#endif
#ifdef HAVE_LIBYAML
    build_info_set_status(BIB_LIB_LIBYAML, true);
#endif
#ifdef HAVE_LIBMNL
    build_info_set_status(BIB_LIB_LIBMNL, true);
#endif

#ifdef ENABLE_PLUGIN_APPS
    build_info_set_status(BIB_PLUGIN_APPS, true);
#endif
#ifdef HAVE_SETNS
    build_info_set_status(BIB_PLUGIN_LINUX_CGROUP_NETWORK, true);
#endif

    build_info_set_status(BIB_PLUGIN_STATSD, true);
    build_info_set_status(BIB_PLUGIN_TIMEX, true);
    build_info_set_status(BIB_PLUGIN_IDLEJITTER, true);
    build_info_set_status(BIB_PLUGIN_BASH, true);

#ifdef ENABLE_PLUGIN_DEBUGFS
    build_info_set_status(BIB_PLUGIN_DEBUGFS, true);
#endif
#ifdef ENABLE_PLUGIN_CUPS
    build_info_set_status(BIB_PLUGIN_CUPS, true);
#endif
#ifdef ENABLE_PLUGIN_EBPF
    build_info_set_status(BIB_PLUGIN_EBPF, true);
#endif
#ifdef ENABLE_PLUGIN_FREEIPMI
    build_info_set_status(BIB_PLUGIN_FREEIPMI, true);
#endif
#ifdef ENABLE_PLUGIN_SYSTEMD_JOURNAL
    build_info_set_status(BIB_PLUGIN_SYSTEMD_JOURNAL, true);
#endif
#ifdef ENABLE_PLUGIN_NETWORK_VIEWER
    build_info_set_status(BIB_PLUGIN_NETWORK_VIEWER, true);
#endif
#ifdef ENABLE_PLUGIN_NFACCT
    build_info_set_status(BIB_PLUGIN_NFACCT, true);
#endif
#ifdef ENABLE_PLUGIN_PERF
    build_info_set_status(BIB_PLUGIN_PERF, true);
#endif
#ifdef ENABLE_PLUGIN_SLABINFO
    build_info_set_status(BIB_PLUGIN_SLABINFO, true);
#endif
#ifdef ENABLE_PLUGIN_XENSTAT
    build_info_set_status(BIB_PLUGIN_XEN, true);
#endif
#ifdef HAVE_XENSTAT_VBD_ERROR
    build_info_set_status(BIB_PLUGIN_XEN_VBD_ERROR, true);
#endif

    build_info_set_status(BIB_EXPORT_PROMETHEUS_EXPORTER, true);
    build_info_set_status(BIB_EXPORT_GRAPHITE, true);
    build_info_set_status(BIB_EXPORT_GRAPHITE_HTTP, true);
    build_info_set_status(BIB_EXPORT_JSON, true);
    build_info_set_status(BIB_EXPORT_JSON_HTTP, true);
    build_info_set_status(BIB_EXPORT_OPENTSDB, true);
    build_info_set_status(BIB_EXPORT_OPENTSDB_HTTP, true);
    build_info_set_status(BIB_EXPORT_ALLMETRICS, true);
    build_info_set_status(BIB_EXPORT_SHELL, true);

#ifdef HAVE_KINESIS
    build_info_set_status(BIB_EXPORT_AWS_KINESIS, true);
#endif
#ifdef ENABLE_EXPORTING_PUBSUB
    build_info_set_status(BIB_EXPORT_GCP_PUBSUB, true);
#endif
#ifdef HAVE_MONGOC
    build_info_set_status(BIB_EXPORT_MONGOC, true);
#endif
#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
    build_info_set_status(BIB_EXPORT_PROMETHEUS_REMOTE_WRITE, true);
#endif

#ifdef NETDATA_TRACE_ALLOCATIONS
    build_info_set_status(BIB_DEVEL_TRACE_ALLOCATIONS, true);
#endif

#if defined(NETDATA_DEV_MODE) || defined(NETDATA_INTERNAL_CHECKS)
    build_info_set_status(BIB_DEVELOPER_MODE, true);
#endif
}

// ----------------------------------------------------------------------------
// system info

static void populate_system_info(void) {
    static bool populated = false;
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    if(populated)
        return;

    spinlock_lock(&spinlock);

    if(populated) {
        spinlock_unlock(&spinlock);
        return;
    }

    struct rrdhost_system_info *system_info;
    bool free_system_info = false;

    if(localhost && localhost->system_info) {
        system_info = localhost->system_info;
    }
    else {
        bool started_spawn_server = false;
        if(!netdata_main_spawn_server) {
            started_spawn_server = true;
            netdata_main_spawn_server_init(NULL, 0, NULL);
        }

        system_info = rrdhost_system_info_create();
        rrdhost_system_info_detect(system_info);
        free_system_info = true;

        if(started_spawn_server)
            netdata_main_spawn_server_cleanup();
    }

    build_info_set_value_strdupz(BIB_OS_KERNEL_NAME, system_info->kernel_name);
    build_info_set_value_strdupz(BIB_OS_KERNEL_VERSION, system_info->kernel_version);
    build_info_set_value_strdupz(BIB_OS_NAME, system_info->host_os_name);
    build_info_set_value_strdupz(BIB_OS_ID, system_info->host_os_id);
    build_info_set_value_strdupz(BIB_OS_ID_LIKE, system_info->host_os_id_like);
    build_info_set_value_strdupz(BIB_OS_VERSION, system_info->host_os_version);
    build_info_set_value_strdupz(BIB_OS_VERSION_ID, system_info->container_os_version_id);
    build_info_set_value_strdupz(BIB_OS_DETECTION, system_info->host_os_detection);
    build_info_set_value_strdupz(BIB_HW_CPU_CORES, system_info->host_cores);
    build_info_set_value_strdupz(BIB_HW_CPU_FREQUENCY, system_info->host_cpu_freq);
    build_info_set_value_strdupz(BIB_HW_RAM_SIZE, system_info->host_ram_total);
    build_info_set_value_strdupz(BIB_HW_DISK_SPACE, system_info->host_disk_space);
    build_info_set_value_strdupz(BIB_HW_ARCHITECTURE, system_info->architecture);
    build_info_set_value_strdupz(BIB_HW_VIRTUALIZATION, system_info->virtualization);
    build_info_set_value_strdupz(BIB_HW_VIRTUALIZATION_DETECTION, system_info->virt_detection);
    build_info_set_value_strdupz(BIB_CONTAINER_NAME, system_info->container);
    build_info_set_value_strdupz(BIB_CONTAINER_DETECTION, system_info->container_detection);

    if(system_info->is_k8s_node && !strcmp(system_info->is_k8s_node, "true"))
        build_info_set_value_strdupz(BIB_CONTAINER_ORCHESTRATOR, "kubernetes");
    else
        build_info_set_value_strdupz(BIB_CONTAINER_ORCHESTRATOR, "none");

    build_info_set_value_strdupz(BIB_CONTAINER_OS_NAME, system_info->container_os_name);
    build_info_set_value_strdupz(BIB_CONTAINER_OS_ID, system_info->container_os_id);
    build_info_set_value_strdupz(BIB_CONTAINER_OS_ID_LIKE, system_info->container_os_id_like);
    build_info_set_value_strdupz(BIB_CONTAINER_OS_VERSION, system_info->container_os_version);
    build_info_set_value_strdupz(BIB_CONTAINER_OS_VERSION_ID, system_info->container_os_version_id);
    build_info_set_value_strdupz(BIB_CONTAINER_OS_DETECTION, system_info->container_os_detection);

    if(free_system_info)
        rrdhost_system_info_free(system_info);

    populated = true;
    spinlock_unlock(&spinlock);
}

// ----------------------------------------------------------------------------
// packaging info

char *get_value_from_key(char *buffer, char *key) {
    char *s = NULL, *t = NULL;
    s = t = buffer + strlen(key) + 2;
    if (s) {
        while (*s == '\'')
            s++;
        while (*++t != '\0');
        while (--t > s && *t == '\'')
            *t = '\0';
    }
    return s;
}

void get_install_type_internal(char **install_type, char **prebuilt_arch __maybe_unused, char **prebuilt_dist __maybe_unused) {
#ifndef OS_WINDOWS
    char *install_type_filename;

    unsigned long install_type_filename_len = (strlen(netdata_configured_user_config_dir) + strlen(".install-type") + 3);
    install_type_filename = mallocz(sizeof(char) * install_type_filename_len);
    snprintfz(install_type_filename, install_type_filename_len - 1, "%s/%s", netdata_configured_user_config_dir, ".install-type");

    FILE *fp = fopen(install_type_filename, "r");
    if (fp) {
        char *s, buf[256 + 1];
        size_t len = 0;

        while ((s = fgets_trim_len(buf, 256, fp, &len))) {
            if (!strncmp(buf, "INSTALL_TYPE='", 14))
                *install_type = strdupz((char *)get_value_from_key(buf, "INSTALL_TYPE"));
            else if (!strncmp(buf, "PREBUILT_ARCH='", 15))
                *prebuilt_arch = strdupz((char *)get_value_from_key(buf, "PREBUILT_ARCH"));
            else if (!strncmp(buf, "PREBUILT_DISTRO='", 17))
                *prebuilt_dist = strdupz((char *)get_value_from_key(buf, "PREBUILT_DISTRO"));
        }
        fclose(fp);
    }
    freez(install_type_filename);
#else
    *install_type = strdupz("netdata_installer.exe");
#endif
}

void get_install_type(struct rrdhost_system_info *system_info) {
    get_install_type_internal(&system_info->install_type, &system_info->prebuilt_arch, &system_info->prebuilt_dist);
}

static struct {
    SPINLOCK spinlock;
    bool populated;
    char *install_type;
    char *prebuilt_arch;
    char *prebuilt_distro;
} BUILD_PACKAGING_INFO = { 0 };

static void populate_packaging_info() {
    if(!BUILD_PACKAGING_INFO.populated) {
        spinlock_lock(&BUILD_PACKAGING_INFO.spinlock);
        if(!BUILD_PACKAGING_INFO.populated) {
            BUILD_PACKAGING_INFO.populated = true;

            get_install_type_internal(&BUILD_PACKAGING_INFO.install_type, &BUILD_PACKAGING_INFO.prebuilt_arch, &BUILD_PACKAGING_INFO.prebuilt_distro);

            if(!BUILD_PACKAGING_INFO.install_type)
                BUILD_PACKAGING_INFO.install_type = "unknown";

            if(!BUILD_PACKAGING_INFO.prebuilt_arch)
                BUILD_PACKAGING_INFO.prebuilt_arch = "unknown";

            if(!BUILD_PACKAGING_INFO.prebuilt_distro)
                BUILD_PACKAGING_INFO.prebuilt_distro = "unknown";

            build_info_set_value(BIB_PACKAGING_INSTALL_TYPE, strdupz(BUILD_PACKAGING_INFO.install_type));
            build_info_set_value(BIB_PACKAGING_ARCHITECTURE, strdupz(BUILD_PACKAGING_INFO.prebuilt_arch));
            build_info_set_value(BIB_PACKAGING_DISTRO, strdupz(BUILD_PACKAGING_INFO.prebuilt_distro));

            CLEAN_BUFFER *wb = buffer_create(0, NULL);
            ND_PROFILE_2buffer(wb, nd_profile_detect_and_configure(false), " ");
            build_info_set_value_strdupz(BIB_RUNTIME_PROFILE, buffer_tostring(wb));

            build_info_set_status(BIB_RUNTIME_PARENT, stream_conf_is_parent(false));
            build_info_set_status(BIB_RUNTIME_CHILD, stream_conf_is_child());
        }
        spinlock_unlock(&BUILD_PACKAGING_INFO.spinlock);
    }

    OS_SYSTEM_MEMORY sm = os_system_memory(true);
    char buf[1024];
    snprintfz(buf, sizeof(buf), "%" PRIu64, sm.ram_total_bytes);
    // size_snprintf(buf, sizeof(buf), sm.ram_total_bytes, "B", false);
    build_info_set_value_strdupz(BIB_RUNTIME_MEM_TOTAL, buf);

    snprintfz(buf, sizeof(buf), "%" PRIu64, sm.ram_available_bytes);
    // size_snprintf(buf, sizeof(buf), sm.ram_available_bytes, "B", false);
    build_info_set_value_strdupz(BIB_RUNTIME_MEM_AVAIL, buf);
}

// ----------------------------------------------------------------------------

static void populate_directories(void) {
    build_info_set_value(BIB_DIR_USER_CONFIG, netdata_configured_user_config_dir);
    build_info_set_value(BIB_DIR_STOCK_CONFIG, netdata_configured_stock_config_dir);
    build_info_set_value(BIB_DIR_CACHE, netdata_configured_cache_dir);
    build_info_set_value(BIB_DIR_LIB, netdata_configured_varlib_dir);
    build_info_set_value(BIB_DIR_PLUGINS, netdata_configured_primary_plugins_dir);
    build_info_set_value(BIB_DIR_WEB, netdata_configured_web_dir);
    build_info_set_value(BIB_DIR_LOG, netdata_configured_log_dir);
    build_info_set_value(BIB_DIR_LOCK, netdata_configured_lock_dir);
    build_info_set_value(BIB_DIR_HOME, netdata_configured_home_dir);
}

// ----------------------------------------------------------------------------

static void print_build_info_category_to_json(BUFFER *b, BUILD_INFO_CATEGORY category, const char *key) {
    buffer_json_member_add_object(b, key);
    for(size_t i = 0; i < BIB_TERMINATOR ;i++) {
        if(BUILD_INFO[i].category == category && BUILD_INFO[i].json) {
            if(BUILD_INFO[i].value)
                buffer_json_member_add_string(b, BUILD_INFO[i].json, BUILD_INFO[i].value);
            else
                buffer_json_member_add_boolean(b, BUILD_INFO[i].json, BUILD_INFO[i].status);
        }
    }
    buffer_json_object_close(b); // key
}

static void print_build_info_category_to_console(BUILD_INFO_CATEGORY category, const char *title) {
    printf("%s:\n", title);
    for(size_t i = 0; i < BIB_TERMINATOR ;i++) {
        if(BUILD_INFO[i].category == category && BUILD_INFO[i].print) {
            const char *v = BUILD_INFO[i].status ? "YES" : "NO";
            const char *k = BUILD_INFO[i].print;
            const char *d = BUILD_INFO[i].value;

            int padding_length = 60 - strlen(k) - 1;
            if (padding_length < 0) padding_length = 0;

            char padding[padding_length + 1];
            memset(padding, '_', padding_length);
            padding[padding_length] = '\0';

            if(BUILD_INFO[i].type == BIT_STRING)
                printf("    %s %s : %s\n", k, padding, d?d:"unknown");
            else
                printf("    %s %s : %s%s%s%s\n", k, padding, v,
                       d?" (":"", d?d:"", d?")":"");
        }
    }
}

void print_build_info(void) {
    populate_packaging_info();
    populate_system_info();
    populate_directories();

    print_build_info_category_to_console(BIC_PACKAGING, "Packaging");
    print_build_info_category_to_console(BIC_DIRECTORIES, "Default Directories");
    print_build_info_category_to_console(BIC_OPERATING_SYSTEM, "Operating System");
    print_build_info_category_to_console(BIC_HARDWARE, "Hardware");
    print_build_info_category_to_console(BIC_CONTAINER, "Container");
    print_build_info_category_to_console(BIC_FEATURE, "Features");
    print_build_info_category_to_console(BIC_DATABASE, "Database Engines");
    print_build_info_category_to_console(BIC_CONNECTIVITY, "Connectivity Capabilities");
    print_build_info_category_to_console(BIC_LIBS, "Libraries");
    print_build_info_category_to_console(BIC_PLUGINS, "Plugins");
    print_build_info_category_to_console(BIC_EXPORTERS, "Exporters");
    print_build_info_category_to_console(BIC_DEBUG_DEVEL, "Debug/Developer Features");
    print_build_info_category_to_console(BIC_RUNTIME, "Runtime Information");
}

void build_info_to_json_object(BUFFER *b) {
    populate_packaging_info();
    populate_system_info();
    populate_directories();

    print_build_info_category_to_json(b, BIC_PACKAGING, "package");
    print_build_info_category_to_json(b, BIC_DIRECTORIES, "directories");
    print_build_info_category_to_json(b, BIC_OPERATING_SYSTEM, "os");
    print_build_info_category_to_json(b, BIC_HARDWARE, "hw");
    print_build_info_category_to_json(b, BIC_CONTAINER, "container");
    print_build_info_category_to_json(b, BIC_FEATURE, "features");
    print_build_info_category_to_json(b, BIC_DATABASE, "databases");
    print_build_info_category_to_json(b, BIC_CONNECTIVITY, "connectivity");
    print_build_info_category_to_json(b, BIC_LIBS, "libs");
    print_build_info_category_to_json(b, BIC_PLUGINS, "plugins");
    print_build_info_category_to_json(b, BIC_EXPORTERS, "exporters");
    print_build_info_category_to_json(b, BIC_DEBUG_DEVEL, "debug-n-devel");
    print_build_info_category_to_json(b, BIC_RUNTIME, "runtime");
}

void print_build_info_json(void) {
    populate_packaging_info();
    populate_system_info();
    populate_directories();

    BUFFER *b = buffer_create(0, NULL);
    buffer_json_initialize(b, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_DEFAULT);

    build_info_to_json_object(b);

    buffer_json_finalize(b);
    printf("%s\n", buffer_tostring(b));
    buffer_free(b);
}

void analytics_build_info(BUFFER *b) {
    populate_packaging_info();
    populate_system_info();
    populate_directories();

    size_t added = 0;
    for(size_t i = 0; i < BIB_TERMINATOR ;i++) {
        if(BUILD_INFO[i].analytics && BUILD_INFO[i].status) {

            if(added)
                buffer_strcat(b, "|");

            buffer_strcat (b, BUILD_INFO[i].analytics);
            added++;
        }
    }
}

void print_build_info_cmake_cache(void) {
        const char *path = NETDATA_RUNTIME_PREFIX "/"
                           BUILD_INFO_CMAKE_CACHE_ARCHIVE_PATH "/"
                           BUILD_INFO_CMAKE_CACHE_ARCHIVE_NAME;

        gzFile f = gzopen(path, "rb");
        if (!f) {
            printf("Could not open build info cmake cache archive located at %s\n",
                   path);
            return;
        }

        char line[1024];
        while (gzgets(f, line, sizeof(line))) {
            printf("%s", line);
        }

        gzclose(f);
}
