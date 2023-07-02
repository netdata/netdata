// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include "./config.h"
#include "common.h"
#include "buildinfo.h"

typedef enum __attribute__((packed)) {
    BIB_PACKAGING_NETDATA_VERSION,
    BIB_PACKAGING_INSTALL_TYPE,
    BIB_PACKAGING_ARCHITECTURE,
    BIB_PACKAGING_DISTRO,
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
    BIB_KUBERNETES,
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
    BIB_FEATURE_REPLICATION,
    BIB_FEATURE_STREAMING_COMPRESSION,
    BIB_FEATURE_CONTEXTS,
    BIB_FEATURE_TIERING,
    BIB_FEATURE_ML,
    BIB_DB_DBENGINE,
    BIB_DB_ALLOC,
    BIB_DB_RAM,
    BIB_DB_MAP,
    BIB_DB_SAVE,
    BIB_DB_NONE,
    BIB_CONNECTIVITY_ACLK,
    BIB_CONNECTIVITY_HTTPD_STATIC,
    BIB_CONNECTIVITY_HTTPD_H2O,
    BIB_CONNECTIVITY_WEBRTC,
    BIB_CONNECTIVITY_NATIVE_HTTPS,
    BIB_CONNECTIVITY_TLS_HOST_VERIFY,
    BIB_LIB_LZ4,
    BIB_LIB_ZLIB,
    BIB_LIB_PROTOBUF,
    BIB_LIB_OPENSSL,
    BIB_LIB_LIBDATACHANNEL,
    BIB_LIB_JSONC,
    BIB_LIB_LIBCAP,
    BIB_LIB_LIBCRYPTO,
    BIB_LIB_LIBM,
    BIB_LIB_JEMALLOC,
    BIB_LIB_TCMALLOC,
    BIB_PLUGIN_APPS,
    BIB_PLUGIN_LINUX_CGROUPS,
    BIB_PLUGIN_LINUX_CGROUP_NETWORK,
    BIB_PLUGIN_LINUX_PROC,
    BIB_PLUGIN_LINUX_TC,
    BIB_PLUGIN_LINUX_DISKSPACE,
    BIB_PLUGIN_FREEBSD,
    BIB_PLUGIN_MACOS,
    BIB_PLUGIN_STATSD,
    BIB_PLUGIN_TIMEX,
    BIB_PLUGIN_IDLEJITTER,
    BIB_PLUGIN_BASH,
    BIB_PLUGIN_DEBUGFS,
    BIB_PLUGIN_CUPS,
    BIB_PLUGIN_EBPF,
    BIB_PLUGIN_FREEIPMI,
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
} BUILD_INFO_BIT;

static BITMAP256 BUILD_INFO = BITMAP256_INITIALIZER; // the bitmap we store our build information

typedef enum __attribute__((packed)) {
    BIC_TERMINATOR = 0,
    BIC_PACKAGING,
    BIC_OPERATING_SYSTEM,
    BIC_HARDWARE,
    BIC_CONTAINER,
    BIC_FEATURE,
    BIC_DATABASE,
    BIC_CONNECTIVITY,
    BIC_LIBS,
    BIC_PLUGINS,
    BIC_EXPORTERS,
    BIC_DEBUG_DEVEL
} BUILD_INFO_CATEGORY;

typedef enum __attribute__((packed)) {
    BIT_BOOLEAN,
    BIT_STRING,
} BUILD_INFO_TYPE;

static struct {
    BUILD_INFO_BIT bit;
    BUILD_INFO_CATEGORY category;
    BUILD_INFO_TYPE type;
    const char *analytics;
    const char *print;
    const char *json;
    const char *value;
} BUILD_INFO_NAMES[] = {
        {
                .bit = BIB_PACKAGING_NETDATA_VERSION,
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Netdata Version",
                .json = "version",
                .value = "unknown",
        },
        {
                .bit = BIB_PACKAGING_INSTALL_TYPE,
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Installation Type",
                .json = "type",
                .value = "unknown",
        },
        {
                .bit = BIB_PACKAGING_ARCHITECTURE,
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Package Architecture",
                .json = "arch",
                .value = "unknown",
        },
        {
                .bit = BIB_PACKAGING_DISTRO,
                .category = BIC_PACKAGING,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Package Distro",
                .json = "distro",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_KERNEL_NAME,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Kernel",
                .json = "kernel",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_KERNEL_VERSION,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Kernel Version",
                .json = "kernel_version",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_NAME,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System",
                .json = "os",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_ID,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System ID",
                .json = "id",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_ID_LIKE,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System ID Like",
                .json = "id_like",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_VERSION,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System Version",
                .json = "version",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_VERSION_ID,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Operating System Version ID",
                .json = "version_id",
                .value = "unknown",
        },
        {
                .bit = BIB_OS_DETECTION,
                .category = BIC_OPERATING_SYSTEM,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Detection",
                .json = "detection",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_CPU_CORES,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "CPU Cores",
                .json = "cpu_cores",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_CPU_FREQUENCY,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "CPU Frequency",
                .json = "cpu_frequency",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_ARCHITECTURE,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "CPU Architecture",
                .json = "cpu_architecture",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_RAM_SIZE,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "RAM Bytes",
                .json = "ram",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_DISK_SPACE,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Disk Capacity",
                .json = "disk",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_VIRTUALIZATION,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Virtualization Technology",
                .json = "virtualization",
                .value = "unknown",
        },
        {
                .bit = BIB_HW_VIRTUALIZATION_DETECTION,
                .category = BIC_HARDWARE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Virtualization Detection",
                .json = "virtualization_detection",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_NAME,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container",
                .json = "container",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_DETECTION,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Detection",
                .json = "container_detection",
                .value = "unknown",
        },
        {
                .bit = BIB_KUBERNETES,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Kubernetes",
                .json = "kubernetes",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_OS_NAME,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System",
                .json = "os",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_OS_ID,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System ID",
                .json = "os_id",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_OS_ID_LIKE,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System ID Like",
                .json = "os_id_like",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_OS_VERSION,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System Version",
                .json = "version",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_OS_VERSION_ID,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System Version ID",
                .json = "version_id",
                .value = "unknown",
        },
        {
                .bit = BIB_CONTAINER_OS_DETECTION,
                .category = BIC_CONTAINER,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Container Operating System Detection",
                .json = "detection",
                .value = "unknown",
        },
        {
                .bit = BIB_FEATURE_BUILT_FOR,
                .category = BIC_FEATURE,
                .type = BIT_STRING,
                .analytics = NULL,
                .print = "Built For",
                .json = "built-for",
                .value = "unknown",
        },
        {
                .bit = BIB_FEATURE_CLOUD,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = "Netdata Cloud",
                .print = "Netdata Cloud",
                .json = "cloud",
                .value = NULL,
        },
        {
                .bit = BIB_FEATURE_HEALTH,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Health (trigger alerts and send notifications)",
                .json = "health",
                .value = NULL,
        },
        {
                .bit = BIB_FEATURE_STREAMING,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Streaming (stream metrics to parent Netdata servers)",
                .json = "streaming",
                .value = NULL,
        },
        {
                .bit = BIB_FEATURE_REPLICATION,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Replication (fill the gaps of parent Netdata servers)",
                .json = "replication",
                .value = NULL,
        },
        {
                .bit = BIB_FEATURE_STREAMING_COMPRESSION,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = "Stream Compression",
                .print = "Streaming and Replication Compression",
                .json = "stream-compression",
                .value = "none",
        },
        {
                .bit = BIB_FEATURE_CONTEXTS,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Contexts (index all active and archived metrics)",
                .json = "contexts",
                .value = NULL,
        },
        {
                .bit = BIB_FEATURE_TIERING,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Tiering (multiple dbs with different metrics resolution)",
                .json = "tiering",
                .value = TOSTRING(RRD_STORAGE_TIERS),
        },
        {
                .bit = BIB_FEATURE_ML,
                .category = BIC_FEATURE,
                .type = BIT_BOOLEAN,
                .analytics = "Machine Learning",
                .print = "Machine Learning",
                .json = "ml",
                .value = NULL,
        },
        {
                .bit = BIB_DB_DBENGINE,
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = "dbengine",
                .print = "dbengine",
                .json = "dbengine",
                .value = NULL,
        },
        {
                .bit = BIB_DB_ALLOC,
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "alloc",
                .json = "alloc",
                .value = NULL,
        },
        {
                .bit = BIB_DB_RAM,
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "ram",
                .json = "ram",
                .value = NULL,
        },
        {
                .bit = BIB_DB_MAP,
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "map",
                .json = "map",
                .value = NULL,
        },
        {
                .bit = BIB_DB_SAVE,
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "save",
                .json = "save",
                .value = NULL,
        },
        {
                .bit = BIB_DB_NONE,
                .category = BIC_DATABASE,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "none",
                .json = "none",
                .value = NULL,
        },
        {
                .bit = BIB_CONNECTIVITY_ACLK,
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "ACLK (Agent-Cloud Link: MQTT over WebSockets over TLS)",
                .json = "aclk",
                .value = NULL,
        },
        {
                .bit = BIB_CONNECTIVITY_HTTPD_STATIC,
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "static (Netdata's internal web server)",
                .json = "static",
                .value = NULL,
        },
        {
                .bit = BIB_CONNECTIVITY_HTTPD_H2O,
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "h2o (web server)",
                .json = "h2o",
                .value = NULL,
        },
        {
                .bit = BIB_CONNECTIVITY_WEBRTC,
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "WebRTC (experimental)",
                .json = "webrtc",
                .value = NULL,
        },
        {
                .bit = BIB_CONNECTIVITY_NATIVE_HTTPS,
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = "Native HTTPS",
                .print = "Native HTTPS (TLS Support)",
                .json = "native-https",
                .value = NULL,
        },
        {
                .bit = BIB_CONNECTIVITY_TLS_HOST_VERIFY,
                .category = BIC_CONNECTIVITY,
                .type = BIT_BOOLEAN,
                .analytics = "TLS Host Verification",
                .print = "TLS Host Verification",
                .json = "tls-host-verify",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_LZ4,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "LZ4",
                .json = "lz4",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_ZLIB,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "zlib",
                .print = "zlib",
                .json = "zlib",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_PROTOBUF,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "protobuf",
                .print = "protobuf",
                .json = "protobuf",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_OPENSSL,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "OpenSSL",
                .json = "openssl",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_LIBDATACHANNEL,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "libdatachannel (WebRTC Data Channels)",
                .json = "libdatachannel",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_JSONC,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "JSON-C",
                .print = "JSON-C",
                .json = "jsonc",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_LIBCAP,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "libcap",
                .print = "libcap",
                .json = "libcap",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_LIBCRYPTO,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "libcrypto",
                .print = "libcrypto",
                .json = "libcrypto",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_LIBM,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "libm",
                .print = "libm",
                .json = "libm",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_JEMALLOC,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "jemalloc",
                .print = "jemalloc",
                .json = "jemalloc",
                .value = NULL,
        },
        {
                .bit = BIB_LIB_TCMALLOC,
                .category = BIC_LIBS,
                .type = BIT_BOOLEAN,
                .analytics = "tcalloc",
                .print = "TCMalloc",
                .json = "tcmalloc",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_APPS,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "apps",
                .print = "apps (monitor processes)",
                .json = "apps",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_LINUX_CGROUPS,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "cgroups (monitor containers and VMs)",
                .json = "cgroups",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_LINUX_CGROUP_NETWORK,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "cgroup Network Tracking",
                .print = "cgroup-network (associate interfaces to CGROUPS)",
                .json = "cgroup-network",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_LINUX_PROC,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "proc (monitor Linux systems)",
                .json = "proc",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_LINUX_TC,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "tc (monitor Linux network QoS)",
                .json = "tc",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_LINUX_DISKSPACE,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "diskspace (monitor Linux mount points)",
                .json = "diskspace",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_FREEBSD,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "freebsd (monitor FreeBSD systems)",
                .json = "freebsd",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_MACOS,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "macos (monitor MacOS systems)",
                .json = "macos",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_STATSD,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "statsd (collect custom application metrics)",
                .json = "statsd",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_TIMEX,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "timex (check system clock synchronization)",
                .json = "timex",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_IDLEJITTER,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "idlejitter (check system latency and jitter)",
                .json = "idlejitter",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_BASH,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "bash (support shell data collection jobs - charts.d)",
                .json = "charts.d",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_DEBUGFS,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "debugfs",
                .print = "debugfs (kernel debugging metrics)",
                .json = "debugfs",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_CUPS,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "CUPS",
                .print = "cups (monitor printers and print jobs)",
                .json = "cups",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_EBPF,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "EBPF",
                .print = "ebpf (monitor system calls)",
                .json = "ebpf",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_FREEIPMI,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "IPMI",
                .print = "freeipmi (monitor enterprise server H/W)",
                .json = "freeipmi",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_NFACCT,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "NFACCT",
                .print = "nfacct (gather netfilter accounting)",
                .json = "nfacct",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_PERF,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "perf",
                .print = "perf (collect kernel performance events)",
                .json = "perf",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_SLABINFO,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "slabinfo",
                .print = "slabinfo (monitor kernel object caching)",
                .json = "slabinfo",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_XEN,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "Xen",
                .print = "Xen",
                .json = "xen",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_XEN_VBD_ERROR,
                .category = BIC_PLUGINS,
                .type = BIT_BOOLEAN,
                .analytics = "Xen VBD Error Tracking",
                .print = "Xen VBD Error Tracking",
                .json = "xen-vbd-error",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_MONGOC,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "MongoDB",
                .print = "MongoDB",
                .json = "mongodb",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_GRAPHITE,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Graphite",
                .json = "graphite",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_GRAPHITE_HTTP,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Graphite HTTP / HTTPS",
                .json = "graphite:http",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_JSON,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "JSON",
                .json = "json",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_JSON_HTTP,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "JSON HTTP / HTTPS",
                .json = "json:http",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_OPENTSDB,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "OpenTSDB",
                .json = "opentsdb",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_OPENTSDB_HTTP,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "OpenTSDB HTTP / HTTPS",
                .json = "opentsdb:http",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_ALLMETRICS,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .type = BIT_BOOLEAN,
                .print = "All Metrics API",
                .json = "allmetrics",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_SHELL,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Shell (use metrics in shell scripts)",
                .json = "shell",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_PROMETHEUS_EXPORTER,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Prometheus (OpenMetrics) Exporter",
                .json = "openmetrics",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_PROMETHEUS_REMOTE_WRITE,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "Prometheus Remote Write",
                .print = "Prometheus Remote Write",
                .json = "prom-remote-write",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_AWS_KINESIS,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "AWS Kinesis",
                .print = "AWS Kinesis",
                .json = "kinesis",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_GCP_PUBSUB,
                .category = BIC_EXPORTERS,
                .type = BIT_BOOLEAN,
                .analytics = "GCP PubSub",
                .print = "GCP PubSub",
                .json = "pubsub",
                .value = NULL,
        },
        {
                .bit = BIB_DEVEL_TRACE_ALLOCATIONS,
                .category = BIC_DEBUG_DEVEL,
                .type = BIT_BOOLEAN,
                .analytics = "DebugTraceAlloc",
                .print = "Trace All Netdata Allocations (with charts)",
                .json = "trace-allocations",
                .value = NULL,
        },
        {
                .bit = BIB_DEVELOPER_MODE,
                .category = BIC_DEBUG_DEVEL,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = "Developer Mode (more runtime checks, slower)",
                .json = "dev-mode",
                .value = NULL,
        },
        {
                .bit = 0,
                .category = BIC_TERMINATOR,
                .type = BIT_BOOLEAN,
                .analytics = NULL,
                .print = NULL,
                .json = NULL,
                .value = NULL,
        },
};

static void build_info_set_value(BUILD_INFO_BIT bit, const char *value) {
    for(size_t i = 0; BUILD_INFO_NAMES[i].category != BIC_TERMINATOR ; i++) {
        if(BUILD_INFO_NAMES[i].bit == bit) {
            BUILD_INFO_NAMES[i].value = value;
            break;
        }
    }
}

static void build_info_set_value_strdupz(BUILD_INFO_BIT bit, const char *value) {
    if(!value) value = "";
    build_info_set_value(bit, strdupz(value));
}

__attribute__((constructor)) void initialize_build_info(void) {
    build_info_set_value(BIB_PACKAGING_NETDATA_VERSION, program_version);

#ifdef COMPILED_FOR_LINUX
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_BUILT_FOR, true);
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "Linux");
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_LINUX_CGROUPS, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_LINUX_PROC, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_LINUX_DISKSPACE, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_LINUX_TC, true);
#endif
#ifdef COMPILED_FOR_FREEBSD
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_BUILT_FOR, true);
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "FreeBSD");
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_FREEBSD, true);
#endif
#ifdef COMPILED_FOR_MACOS
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_BUILT_FOR, true);
    build_info_set_value(BIB_FEATURE_BUILT_FOR, "MacOS");
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_MACOS, true);
#endif

#ifdef ENABLE_ACLK
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_CLOUD, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_CONNECTIVITY_ACLK, true);
#else
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_CLOUD, false);
#ifdef DISABLE_CLOUD
    build_info_set_value(BIB_FEATURE_CLOUD, "disabled");
#else
    build_info_set_value(BIB_FEATURE_CLOUD, "unavailable");
#endif
#endif

    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_HEALTH, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_STREAMING, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_REPLICATION, true);

#ifdef ENABLE_RRDPUSH_COMPRESSION
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_STREAMING_COMPRESSION, true);
#ifdef ENABLE_LZ4
    build_info_set_value(BIB_FEATURE_STREAMING_COMPRESSION, "lz4");
#endif
#endif

    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_CONTEXTS, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_TIERING, true);

#ifdef ENABLE_ML
    bitmap256_set_bit(&BUILD_INFO, BIB_FEATURE_ML, true);
#endif

#ifdef ENABLE_DBENGINE
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_DBENGINE, true);
#endif
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_ALLOC, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_RAM, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_MAP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_SAVE, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_NONE, true);

    bitmap256_set_bit(&BUILD_INFO, BIB_CONNECTIVITY_HTTPD_STATIC, true);
#ifdef ENABLE_H2O
    bitmap256_set_bit(&BUILD_INFO, BIB_CONNECTIVITY_HTTPD_H2O, true);
#endif
#ifdef ENABLE_WEBRTC
    bitmap256_set_bit(&BUILD_INFO, BIB_CONNECTIVITY_WEBRTC, true);
#endif
#ifdef ENABLE_HTTPS
    bitmap256_set_bit(&BUILD_INFO, BIB_CONNECTIVITY_NATIVE_HTTPS, true);
#endif
#if defined(HAVE_X509_VERIFY_PARAM_set1_host) && HAVE_X509_VERIFY_PARAM_set1_host == 1
    bitmap256_set_bit(&BUILD_INFO, BIB_CONNECTIVITY_TLS_HOST_VERIFY, true);
#endif

#ifdef ENABLE_LZ4
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_LZ4, true);
#endif

    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_ZLIB, true);

#ifdef HAVE_PROTOBUF
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_PROTOBUF, true);
#ifdef BUNDLED_PROTOBUF
    build_info_set_value(BIB_LIB_PROTOBUF, "bundled");
#else
    build_info_set_value(BIB_LIB_PROTOBUF, "system");
#endif
#endif

#ifdef HAVE_LIBDATACHANNEL
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_LIBDATACHANNEL, true);
#endif
#ifdef ENABLE_OPENSSL
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_OPENSSL, true);
#endif
#ifdef ENABLE_JSONC
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_JSONC, true);
#endif
#ifdef HAVE_CAPABILITY
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_LIBCAP, true);
#endif
#ifdef HAVE_CRYPTO
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_LIBCRYPTO, true);
#endif
#ifdef STORAGE_WITH_MATH
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_LIBM, true);
#endif
#ifdef ENABLE_JEMALLOC
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_JEMALLOC, true);
#endif
#ifdef ENABLE_TCMALLOC
    bitmap256_set_bit(&BUILD_INFO, BIB_LIB_TCMALLOC, true);
#endif

#ifdef ENABLE_APPS_PLUGIN
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_APPS, true);
#endif
#ifdef HAVE_SETNS
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_LINUX_CGROUP_NETWORK, true);
#endif

    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_STATSD, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_TIMEX, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_IDLEJITTER, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_BASH, true);

#ifdef ENABLE_DEBUGFS_PLUGIN
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_DEBUGFS, true);
#endif
#ifdef HAVE_CUPS
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_CUPS, true);
#endif
#ifdef HAVE_LIBBPF
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_EBPF, true);
#endif
#ifdef HAVE_FREEIPMI
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_FREEIPMI, true);
#endif
#ifdef HAVE_NFACCT
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_NFACCT, true);
#endif
#ifdef ENABLE_PERF_PLUGIN
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_PERF, true);
#endif
#ifdef ENABLE_SLABINFO
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_SLABINFO, true);
#endif
#ifdef HAVE_LIBXENSTAT
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_XEN, true);
#endif
#ifdef HAVE_XENSTAT_VBD_ERROR
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_XEN_VBD_ERROR, true);
#endif

    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_PROMETHEUS_EXPORTER, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_GRAPHITE, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_GRAPHITE_HTTP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_JSON, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_JSON_HTTP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_OPENTSDB, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_OPENTSDB_HTTP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_ALLMETRICS, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_SHELL, true);

#ifdef HAVE_KINESIS
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_AWS_KINESIS, true);
#endif
#ifdef ENABLE_EXPORTING_PUBSUB
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_GCP_PUBSUB, true);
#endif
#ifdef HAVE_MONGOC
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_MONGOC, true);
#endif
#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_PROMETHEUS_REMOTE_WRITE, true);
#endif

#ifdef NETDATA_TRACE_ALLOCATIONS
    bitmap256_set_bit(&BUILD_INFO, BIB_DEVEL_TRACE_ALLOCATIONS, true);
#endif

#if defined(NETDATA_DEV_MODE) || defined(NETDATA_INTERNAL_CHECKS)
    bitmap256_set_bit(&BUILD_INFO, BIB_DEVELOPER_MODE, true);
#endif
}

// ----------------------------------------------------------------------------
// system info

int get_system_info(struct rrdhost_system_info *system_info, bool log);
static void populate_system_info(void) {
    static bool populated = false;
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;

    if(populated)
        return;

    spinlock_lock(&spinlock);

    if(populated) {
        spinlock_unlock(&spinlock);
        return;
    }

    struct rrdhost_system_info *system_info;
    bool free_system_info = false;

    if(localhost && !localhost->system_info) {
        system_info = localhost->system_info;
    }
    else {
        system_info = callocz(1, sizeof(struct rrdhost_system_info));
        get_system_info(system_info, false);
        free_system_info = true;
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
    build_info_set_value_strdupz(BIB_KUBERNETES, system_info->is_k8s_node);
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

void get_install_type(char **install_type, char **prebuilt_arch, char **prebuilt_dist) {
    char *install_type_filename;

    int install_type_filename_len = (strlen(netdata_configured_user_config_dir) + strlen(".install-type") + 3);
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

            get_install_type(&BUILD_PACKAGING_INFO.install_type, &BUILD_PACKAGING_INFO.prebuilt_arch, &BUILD_PACKAGING_INFO.prebuilt_distro);

            if(!BUILD_PACKAGING_INFO.install_type)
                BUILD_PACKAGING_INFO.install_type = "unknown";

            if(!BUILD_PACKAGING_INFO.prebuilt_arch)
                BUILD_PACKAGING_INFO.prebuilt_arch = "unknown";

            if(!BUILD_PACKAGING_INFO.prebuilt_distro)
                BUILD_PACKAGING_INFO.prebuilt_distro = "unknown";

            build_info_set_value(BIB_PACKAGING_INSTALL_TYPE, strdupz(BUILD_PACKAGING_INFO.install_type));
            build_info_set_value(BIB_PACKAGING_ARCHITECTURE, strdupz(BUILD_PACKAGING_INFO.prebuilt_arch));
            build_info_set_value(BIB_PACKAGING_DISTRO, strdupz(BUILD_PACKAGING_INFO.prebuilt_distro));
        }
        spinlock_unlock(&BUILD_PACKAGING_INFO.spinlock);
    }
}

// ----------------------------------------------------------------------------

static void print_build_info_category_to_json(BUFFER *b, BUILD_INFO_CATEGORY category, const char *key) {
    buffer_json_member_add_object(b, key);
    for(size_t i = 0; BUILD_INFO_NAMES[i].category != BIC_TERMINATOR ;i++) {
        if(BUILD_INFO_NAMES[i].category == category && BUILD_INFO_NAMES[i].json) {
            if(BUILD_INFO_NAMES[i].value)
                buffer_json_member_add_string(b, BUILD_INFO_NAMES[i].json, BUILD_INFO_NAMES[i].value);
            else
                buffer_json_member_add_boolean(b, BUILD_INFO_NAMES[i].json, bitmap256_get_bit(&BUILD_INFO, BUILD_INFO_NAMES[i].bit));
        }
    }
    buffer_json_object_close(b); // key
}

static void print_build_info_category_to_console(BUILD_INFO_CATEGORY category, const char *title) {
    printf("%s:\n", title);
    for(size_t i = 0; BUILD_INFO_NAMES[i].category != BIC_TERMINATOR ;i++) {
        if(BUILD_INFO_NAMES[i].category == category && BUILD_INFO_NAMES[i].print) {
            const char *v = bitmap256_get_bit(&BUILD_INFO, BUILD_INFO_NAMES[i].bit) ? "YES" : "NO";
            const char *k = BUILD_INFO_NAMES[i].print;
            const char *d = BUILD_INFO_NAMES[i].value;

            int padding_length = 60 - strlen(k) - 1;
            if (padding_length < 0) padding_length = 0;

            char padding[padding_length + 1];
            memset(padding, '_', padding_length);
            padding[padding_length] = '\0';

            if(BUILD_INFO_NAMES[i].type == BIT_STRING)
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

    print_build_info_category_to_console(BIC_PACKAGING, "Packaging");
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
};

void build_info_to_json_object(BUFFER *b) {
    populate_packaging_info();
    populate_system_info();

    print_build_info_category_to_json(b, BIC_PACKAGING, "package");
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
}

void print_build_info_json(void) {
    populate_packaging_info();
    populate_system_info();

    BUFFER *b = buffer_create(0, NULL);
    buffer_json_initialize(b, "\"", "\"", 0, true, false);

    build_info_to_json_object(b);

    buffer_json_finalize(b);
    printf("%s\n", buffer_tostring(b));
    buffer_free(b);
};

void analytics_build_info(BUFFER *b) {
    populate_packaging_info();
    populate_system_info();

    size_t added = 0;
    for(size_t i = 0; BUILD_INFO_NAMES[i].category != BIC_TERMINATOR ;i++) {
        if(BUILD_INFO_NAMES[i].analytics && bitmap256_get_bit(&BUILD_INFO, BUILD_INFO_NAMES[i].bit)) {

            if(added)
                buffer_strcat(b, "|");

            buffer_strcat (b, BUILD_INFO_NAMES[i].analytics);
            added++;
        }
    }
}

