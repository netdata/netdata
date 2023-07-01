// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include "./config.h"
#include "common.h"
#include "buildinfo.h"

typedef enum __attribute__((packed)) {
    BIB_DB_RAM = 0,
    BIB_DB_SAVE,
    BIB_DB_MAP,
    BIB_DB_ALLOC,
    BIB_DB_NONE,
    BIB_DB_DBENGINE,
    BIB_HTTPD_STATIC,
    BIB_HTTPD_H2O,
    BIB_WEBRTC,
    BIB_NATIVE_HTTPS,
    BIB_ACLK,
    BIB_CLOUD,
    BIB_CLOUD_DISABLED,
    BIB_LZ4,
    BIB_ZLIB,
    BIB_PROTOBUF,
    BIB_PROTOBUF_SOURCE,
    BIB_JEMALLOC,
    BIB_TCMALLOC,
    BIB_JSONC,
    BIB_LIBCAP,
    BIB_LIBCRYPTO,
    BIB_LIBM,
    BIB_SETNS,
    BIB_TRACE_ALLOCATIONS,
    BIB_TLS_HOST_VERIFY,
    BIB_ML,
    BIB_PLUGIN_APPS,
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
} BUILD_INFO_BIT;

static BITMAP256 BUILD_INFO = { 0 }; // the bitmap we store our build information

typedef enum __attribute__((packed)) {
    BIC_TERMINATOR = 0,
    BIC_FEATURE,
    BIC_DATABASE,
    BIC_CONNECTIVITY,
    BIC_LIBS,
    BIC_PLUGINS,
    BIC_EXPORTERS,
    BIC_DEBUG_DEVEL
} BUILD_INFO_CATEGORY;

static struct {
    BUILD_INFO_BIT bit;
    BUILD_INFO_CATEGORY category;
    const char *analytics;
    const char *print;
    const char *print_json;
    const char *value;
} BUILD_INFO_NAMES[] = {
        {
                .bit = BIB_DB_DBENGINE,
                .category = BIC_DATABASE,
                .analytics = "dbengine",
                .print = "dbengine",
                .print_json = "dbengine",
                .value = NULL,
        },
        {
                .bit = BIB_DB_ALLOC,
                .category = BIC_DATABASE,
                .analytics = NULL,
                .print = "alloc",
                .print_json = "alloc",
                .value = NULL,
        },
        {
                .bit = BIB_DB_RAM,
                .category = BIC_DATABASE,
                .analytics = NULL,
                .print = "ram",
                .print_json = "ram",
                .value = NULL,
        },
        {
                .bit = BIB_DB_MAP,
                .category = BIC_DATABASE,
                .analytics = NULL,
                .print = "map",
                .print_json = "map",
                .value = NULL,
        },
        {
                .bit = BIB_DB_SAVE,
                .category = BIC_DATABASE,
                .analytics = NULL,
                .print = "save",
                .print_json = "save",
                .value = NULL,
        },
        {
                .bit = BIB_DB_NONE,
                .category = BIC_DATABASE,
                .analytics = NULL,
                .print = "none",
                .print_json = "none",
                .value = NULL,
        },
        {
                .bit = BIB_CLOUD,
                .category = BIC_FEATURE,
                .analytics = "Netdata Cloud",
                .print = "Netdata Cloud",
                .print_json = "cloud",
                .value = NULL,
        },
        {
                .bit = BIB_CLOUD_DISABLED,
                .category = BIC_FEATURE,
                .analytics = NULL,
                .print = NULL,
                .print_json = "cloud-disabled",
                .value = NULL,
        },
        {
                .bit = BIB_ACLK,
                .category = BIC_CONNECTIVITY,
                .analytics = NULL,
                .print = "ACLK (Agent-Cloud Link: MQTT over WebSockets over TLS)",
                .print_json = "aclk",
                .value = NULL,
        },
        {
                .bit = BIB_HTTPD_STATIC,
                .category = BIC_CONNECTIVITY,
                .analytics = NULL,
                .print = "static (Netdata's internal web server)",
                .print_json = "static",
                .value = NULL,
        },
        {
                .bit = BIB_HTTPD_H2O,
                .category = BIC_CONNECTIVITY,
                .analytics = NULL,
                .print = "h2o (web server)",
                .print_json = "h2o",
                .value = NULL,
        },
        {
                .bit = BIB_WEBRTC,
                .category = BIC_CONNECTIVITY,
                .analytics = NULL,
                .print = "WebRTC (experimental)",
                .print_json = "webrtc",
                .value = NULL,
        },
        {
                .bit = BIB_NATIVE_HTTPS,
                .category = BIC_CONNECTIVITY,
                .analytics = "Native HTTPS",
                .print = "Native HTTPS (TLS Support)",
                .print_json = "native-https",
                .value = NULL,
        },
        {
                .bit = BIB_TLS_HOST_VERIFY,
                .category = BIC_CONNECTIVITY,
                .analytics = "TLS Host Verification",
                .print = "TLS Host Verification",
                .print_json = "tls-host-verify",
                .value = NULL,
        },
        {
                .bit = BIB_ML,
                .category = BIC_FEATURE,
                .analytics = "Machine Learning",
                .print = "Machine Learning",
                .print_json = "machine-learning",
                .value = NULL,
        },
        {
                .bit = BIB_LZ4,
                .category = BIC_FEATURE,
                .analytics = "Stream Compression",
                .print = "Stream Compression (LZ4)",
                .print_json = "stream-compression",
                .value = NULL,
        },
        {
                .bit = BIB_PROTOBUF,
                .category = BIC_LIBS,
                .analytics = "protobuf",
                .print = "protobuf",
                .print_json = "protobuf",
                .value = NULL,
        },
        {
                .bit = BIB_PROTOBUF_SOURCE,
                .category = BIC_LIBS,
                .analytics = NULL,
                .print = "protobuf-source",
                .print_json = "protobuf-source",
                .value = "none",
        },
        {
                .bit = BIB_JSONC,
                .category = BIC_LIBS,
                .analytics = "JSON-C",
                .print = "JSON-C",
                .print_json = "jsonc",
                .value = NULL,
        },
        {
                .bit = BIB_LIBCAP,
                .category = BIC_LIBS,
                .analytics = "libcap",
                .print = "libcap",
                .print_json = "libcap",
                .value = NULL,
        },
        {
                .bit = BIB_LIBCRYPTO,
                .category = BIC_LIBS,
                .analytics = "libcrypto",
                .print = "libcrypto",
                .print_json = "libcrypto",
                .value = NULL,
        },
        {
                .bit = BIB_LIBM,
                .category = BIC_LIBS,
                .analytics = "libm",
                .print = "libm",
                .print_json = "libm",
                .value = NULL,
        },
        {
                .bit = BIB_JEMALLOC,
                .category = BIC_LIBS,
                .analytics = "jemalloc",
                .print = "jemalloc",
                .print_json = "jemalloc",
                .value = NULL,
        },
        {
                .bit = BIB_TCMALLOC,
                .category = BIC_LIBS,
                .analytics = "tcalloc",
                .print = "TCMalloc",
                .print_json = "tcmalloc",
                .value = NULL,
        },
        {
                .bit = BIB_ZLIB,
                .category = BIC_LIBS,
                .analytics = "zlib",
                .print = "zlib",
                .print_json = "zlib",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_APPS,
                .category = BIC_PLUGINS,
                .analytics = "apps",
                .print = "apps (monitor processes)",
                .print_json = "apps",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_DEBUGFS,
                .category = BIC_PLUGINS,
                .analytics = "debugfs",
                .print = "debugfs (kernel debugging metrics)",
                .print_json = "debugfs",
                .value = NULL,
        },
        {
                .bit = BIB_SETNS,
                .category = BIC_PLUGINS,
                .analytics = "cgroup Network Tracking",
                .print = "CGROUP Network with setns()",
                .print_json = "cgroup-net",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_CUPS,
                .category = BIC_PLUGINS,
                .analytics = "CUPS",
                .print = "CUPS (monitor printers and print jobs)",
                .print_json = "cups",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_EBPF,
                .category = BIC_PLUGINS,
                .analytics = "EBPF",
                .print = "eBPF (monitor system calls)",
                .print_json = "ebpf",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_FREEIPMI,
                .category = BIC_PLUGINS,
                .analytics = "IPMI",
                .print = "FreeIPMI (monitor server H/W)",
                .print_json = "freeipmi",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_NFACCT,
                .category = BIC_PLUGINS,
                .analytics = "NFACCT",
                .print = "NFACCT (gather netfilter accounting)",
                .print_json = "nfacct",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_PERF,
                .category = BIC_PLUGINS,
                .analytics = "perf",
                .print = "Perf (collect kernel performance events)",
                .print_json = "perf",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_SLABINFO,
                .category = BIC_PLUGINS,
                .analytics = "slabinfo",
                .print = "slabinfo (monitor kernel object caching)",
                .print_json = "slabinfo",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_XEN,
                .category = BIC_PLUGINS,
                .analytics = "Xen",
                .print = "Xen",
                .print_json = "xen",
                .value = NULL,
        },
        {
                .bit = BIB_PLUGIN_XEN_VBD_ERROR,
                .category = BIC_PLUGINS,
                .analytics = "Xen VBD Error Tracking",
                .print = "Xen VBD Error Tracking",
                .print_json = "xen-vbd-error",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_MONGOC,
                .category = BIC_EXPORTERS,
                .analytics = "MongoDB",
                .print = "MongoDB",
                .print_json = "mongodb",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_GRAPHITE,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "Graphite",
                .print_json = "graphite",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_GRAPHITE_HTTP,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "Graphite HTTP / HTTPS",
                .print_json = "graphite:http",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_JSON,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "JSON",
                .print_json = "json",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_JSON_HTTP,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "JSON HTTP / HTTPS",
                .print_json = "json:http",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_OPENTSDB,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "OpenTSDB",
                .print_json = "opentsdb",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_OPENTSDB_HTTP,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "OpenTSDB HTTP / HTTPS",
                .print_json = "opentsdb:http",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_ALLMETRICS,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "All Metrics API",
                .print_json = "allmetrics",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_SHELL,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "Shell (use metrics in shell scripts)",
                .print_json = "shell",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_PROMETHEUS_EXPORTER,
                .category = BIC_EXPORTERS,
                .analytics = NULL,
                .print = "Prometheus (OpenMetrics) Exporter",
                .print_json = "openmetrics",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_PROMETHEUS_REMOTE_WRITE,
                .category = BIC_EXPORTERS,
                .analytics = "Prometheus Remote Write",
                .print = "Prometheus Remote Write",
                .print_json = "prom-remote-write",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_AWS_KINESIS,
                .category = BIC_EXPORTERS,
                .analytics = "AWS Kinesis",
                .print = "AWS Kinesis",
                .print_json = "kinesis",
                .value = NULL,
        },
        {
                .bit = BIB_EXPORT_GCP_PUBSUB,
                .category = BIC_EXPORTERS,
                .analytics = "GCP PubSub",
                .print = "GCP PubSub",
                .print_json = "pubsub",
                .value = NULL,
        },
        {
                .bit = BIB_TRACE_ALLOCATIONS,
                .category = BIC_DEBUG_DEVEL,
                .analytics = "DebugTraceAlloc",
                .print = "Trace All Netdata Allocations (with charts)",
                .print_json = "trace-allocations",
                .value = NULL,
        },
        {
                .bit = 0,
                .category = BIC_TERMINATOR,
                .analytics = NULL,
                .print = NULL,
                .print_json = NULL,
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

__attribute__((constructor)) void initialize_build_info(void) {
    bitmap256_set_bit(&BUILD_INFO, BIB_ZLIB, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_HTTPD_STATIC, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_ALLOC, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_MAP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_NONE, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_SAVE, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_RAM, true);

    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_PROMETHEUS_EXPORTER, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_GRAPHITE, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_GRAPHITE_HTTP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_JSON, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_JSON_HTTP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_OPENTSDB, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_OPENTSDB_HTTP, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_ALLMETRICS, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_EXPORT_SHELL, true);

#ifdef ENABLE_DBENGINE
    bitmap256_set_bit(&BUILD_INFO, BIB_DB_DBENGINE, true);
#endif
#ifdef HAVE_LIBDATACHANNEL
    bitmap256_set_bit(&BUILD_INFO, BIB_WEBRTC, true);
#endif
#ifdef ENABLE_H2O
    bitmap256_set_bit(&BUILD_INFO, BIB_HTTPD_H2O, true);
#endif
#ifdef ENABLE_HTTPS
    bitmap256_set_bit(&BUILD_INFO, BIB_NATIVE_HTTPS, true);
#endif

#ifdef ENABLE_ACLK
    bitmap256_set_bit(&BUILD_INFO, BIB_CLOUD, true);
    bitmap256_set_bit(&BUILD_INFO, BIB_ACLK, true);
#else
    bitmap256_set_bit(&BUILD_INFO, BIB_CLOUD_DISABLED, true);
#ifdef DISABLE_CLOUD
    build_info_set_value(BIB_CLOUD_DISABLED, "disabled by user");
#else
    build_info_set_value(BIB_CLOUD_DISABLED, "not available");
#endif
#endif

#if defined(HAVE_X509_VERIFY_PARAM_set1_host) && HAVE_X509_VERIFY_PARAM_set1_host == 1
    bitmap256_set_bit(&BUILD_INFO, BIB_TLS_HOST_VERIFY, true);
#endif
#ifdef ENABLE_ML
    bitmap256_set_bit(&BUILD_INFO, BIB_ML, true);
#endif
#ifdef ENABLE_LZ4
    bitmap256_set_bit(&BUILD_INFO, BIB_LZ4, true);
#endif

#ifdef HAVE_PROTOBUF
    bitmap256_set_bit(&BUILD_INFO, BIB_PROTOBUF, true);
#ifdef BUNDLED_PROTOBUF
    build_info_set_value(BIB_PROTOBUF_SOURCE, "bundled");
#else
    build_info_set_value(BIB_PROTOBUF_SOURCE, "system");
#endif
#endif

#ifdef ENABLE_JEMALLOC
    bitmap256_set_bit(&BUILD_INFO, BIB_JEMALLOC, true);
#endif
#ifdef ENABLE_JSONC
    bitmap256_set_bit(&BUILD_INFO, BIB_JSONC, true);
#endif
#ifdef HAVE_CAPABILITY
    bitmap256_set_bit(&BUILD_INFO, BIB_LIBCAP, true);
#endif
#ifdef HAVE_CRYPTO
    bitmap256_set_bit(&BUILD_INFO, BIB_LIBCRYPTO, true);
#endif
#ifdef STORAGE_WITH_MATH
    bitmap256_set_bit(&BUILD_INFO, BIB_LIBM, true);
#endif
#ifdef ENABLE_TCMALLOC
    bitmap256_set_bit(&BUILD_INFO, BIB_TCMALLOC, true);
#endif
#ifdef ENABLE_APPS_PLUGIN
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_APPS, true);
#endif
#ifdef ENABLE_DEBUGFS_PLUGIN
    bitmap256_set_bit(&BUILD_INFO, BIB_PLUGIN_DEBUGFS, true);
#endif
#ifdef HAVE_SETNS
    bitmap256_set_bit(&BUILD_INFO, BIB_SETNS, true);
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
    bitmap256_set_bit(&BUILD_INFO, BIB_TRACE_ALLOCATIONS, true);
#endif
}

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

static void print_build_info_category_to_json(BUFFER *b, BUILD_INFO_CATEGORY category, const char *key) {
    buffer_json_member_add_object(b, key);
    for(size_t i = 0; BUILD_INFO_NAMES[i].category != BIC_TERMINATOR ;i++) {
        if(BUILD_INFO_NAMES[i].category == category && BUILD_INFO_NAMES[i].print_json) {
            if(BUILD_INFO_NAMES[i].value)
                buffer_json_member_add_string(b, BUILD_INFO_NAMES[i].print_json, BUILD_INFO_NAMES[i].value);
            else
                buffer_json_member_add_boolean(b, BUILD_INFO_NAMES[i].print_json,bitmap256_get_bit(&BUILD_INFO, BUILD_INFO_NAMES[i].bit));
        }
    }
    buffer_json_object_close(b); // key
}

static void print_build_info_category_to_console(BUILD_INFO_CATEGORY category, const char *title) {
    printf("%s:\n", title);
    for(size_t i = 0; BUILD_INFO_NAMES[i].category != BIC_TERMINATOR ;i++) {
        if(BUILD_INFO_NAMES[i].category == category && BUILD_INFO_NAMES[i].print) {
            const char *v, *k = BUILD_INFO_NAMES[i].print;
            if(BUILD_INFO_NAMES[i].value)
                v = BUILD_INFO_NAMES[i].value;
            else
                v = bitmap256_get_bit(&BUILD_INFO, BUILD_INFO_NAMES[i].bit) ? "YES" : "NO";


            int padding_length = 60 - strlen(k) - 1;
            if (padding_length < 0) padding_length = 0;

            char padding[padding_length + 1];
            memset(padding, '_', padding_length);
            padding[padding_length] = '\0';

            printf("    %s %s : %s\n", k, padding, v);
        }
    }
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
        }
        spinlock_unlock(&BUILD_PACKAGING_INFO.spinlock);
    }
}

void print_build_info(void) {
    populate_packaging_info();
    printf("Configure options: %s\n", CONFIGURE_COMMAND);
    printf("Install type: %s\n", BUILD_PACKAGING_INFO.install_type);
    printf("    Binary architecture: %s\n", BUILD_PACKAGING_INFO.prebuilt_arch);
    printf("    Packaging distro: %s\n", BUILD_PACKAGING_INFO.prebuilt_distro);

    print_build_info_category_to_console(BIC_FEATURE, "Features");
    print_build_info_category_to_console(BIC_DATABASE, "Database Engines");
    print_build_info_category_to_console(BIC_CONNECTIVITY, "Connectivity Capabilities");
    print_build_info_category_to_console(BIC_LIBS, "Libraries");
    print_build_info_category_to_console(BIC_PLUGINS, "Plugins");
    print_build_info_category_to_console(BIC_EXPORTERS, "Exporters");
    print_build_info_category_to_console(BIC_DEBUG_DEVEL, "Debug/Developer Features");
};

void build_info_to_json_object(BUFFER *b) {
    buffer_json_member_add_object(b, "packaging");
    {
        populate_packaging_info();
        buffer_json_member_add_string(b, "configure_options", CONFIGURE_COMMAND);
        buffer_json_member_add_string(b, "install_type", BUILD_PACKAGING_INFO.install_type);
        buffer_json_member_add_string(b, "binary_architecture", BUILD_PACKAGING_INFO.prebuilt_arch);
        buffer_json_member_add_string(b, "packaging_distro", BUILD_PACKAGING_INFO.prebuilt_distro);
    }
    buffer_json_object_close(b);

    print_build_info_category_to_json(b, BIC_FEATURE, "features");
    print_build_info_category_to_json(b, BIC_DATABASE, "databases");
    print_build_info_category_to_json(b, BIC_CONNECTIVITY, "connectivity");
    print_build_info_category_to_json(b, BIC_LIBS, "libs");
    print_build_info_category_to_json(b, BIC_PLUGINS, "plugins");
    print_build_info_category_to_json(b, BIC_EXPORTERS, "exporters");
    print_build_info_category_to_json(b, BIC_DEBUG_DEVEL, "debug-n-devel");
}

void print_build_info_json(void) {
    BUFFER *b = buffer_create(0, NULL);
    buffer_json_initialize(b, "\"", "\"", 0, true, false);

    build_info_to_json_object(b);

    buffer_json_finalize(b);
    printf("%s\n", buffer_tostring(b));
    buffer_free(b);
};

void analytics_build_info(BUFFER *b) {
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

