// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include "./config.h"
#include "common.h"
#include "buildinfo.h"

// Optional features

#ifdef ENABLE_ACLK
#define FEAT_CLOUD 1
#define FEAT_CLOUD_MSG ""
#else
#ifdef DISABLE_CLOUD
#define FEAT_CLOUD 0
#define FEAT_CLOUD_MSG "(by user request)"
#else
#define FEAT_CLOUD 0
#define FEAT_CLOUD_MSG ""
#endif
#endif

#ifdef ENABLE_HTTPD
#define FEAT_HTTPD 1
#else
#define FEAT_HTTPD 0
#endif

#ifdef ENABLE_DBENGINE
#define FEAT_DBENGINE 1
#else
#define FEAT_DBENGINE 0
#endif

#if defined(HAVE_X509_VERIFY_PARAM_set1_host) && HAVE_X509_VERIFY_PARAM_set1_host == 1
#define FEAT_TLS_HOST_VERIFY 1
#else
#define FEAT_TLS_HOST_VERIFY 0
#endif

#ifdef ENABLE_HTTPS
#define FEAT_NATIVE_HTTPS 1
#else
#define FEAT_NATIVE_HTTPS 0
#endif

#ifdef ENABLE_ML
#define FEAT_ML 1
#else
#define FEAT_ML 0
#endif

#ifdef  ENABLE_COMPRESSION
#define  FEAT_STREAM_COMPRESSION 1
#else
#define  FEAT_STREAM_COMPRESSION 0
#endif  //ENABLE_COMPRESSION


// Optional libraries

#ifdef HAVE_PROTOBUF
#define FEAT_PROTOBUF 1
#ifdef BUNDLED_PROTOBUF
#define FEAT_PROTOBUF_BUNDLED " (bundled)"
#else
#define FEAT_PROTOBUF_BUNDLED " (system)"
#endif
#else
#define FEAT_PROTOBUF 0
#define FEAT_PROTOBUF_BUNDLED ""
#endif

#ifdef ENABLE_JSONC
#define FEAT_JSONC 1
#else
#define FEAT_JSONC 0
#endif

#ifdef ENABLE_JEMALLOC
#define FEAT_JEMALLOC 1
#else
#define FEAT_JEMALLOC 0
#endif

#ifdef ENABLE_TCMALLOC
#define FEAT_TCMALLOC 1
#else
#define FEAT_TCMALLOC 0
#endif

#ifdef HAVE_CAPABILITY
#define FEAT_LIBCAP 1
#else
#define FEAT_LIBCAP 0
#endif

#ifdef STORAGE_WITH_MATH
#define FEAT_LIBM 1
#else
#define FEAT_LIBM 0
#endif

#ifdef HAVE_CRYPTO
#define FEAT_CRYPTO 1
#else
#define FEAT_CRYPTO 0
#endif

// Optional plugins

#ifdef ENABLE_APPS_PLUGIN
#define FEAT_APPS_PLUGIN 1
#else
#define FEAT_APPS_PLUGIN 0
#endif

#ifdef ENABLE_DEBUGFS_PLUGIN
#define FEAT_DEBUGFS_PLUGIN 1
#else
#define FEAT_DEBUGFS_PLUGIN 0
#endif

#ifdef HAVE_FREEIPMI
#define FEAT_IPMI 1
#else
#define FEAT_IPMI 0
#endif

#ifdef HAVE_CUPS
#define FEAT_CUPS 1
#else
#define FEAT_CUPS 0
#endif

#ifdef HAVE_NFACCT
#define FEAT_NFACCT 1
#else
#define FEAT_NFACCT 0
#endif

#ifdef HAVE_LIBXENSTAT
#define FEAT_XEN 1
#else
#define FEAT_XEN 0
#endif

#ifdef HAVE_XENSTAT_VBD_ERROR
#define FEAT_XEN_VBD_ERROR 1
#else
#define FEAT_XEN_VBD_ERROR 0
#endif

#ifdef HAVE_LIBBPF
#define FEAT_EBPF 1
#else
#define FEAT_EBPF 0
#endif

#ifdef HAVE_SETNS
#define FEAT_CGROUP_NET 1
#else
#define FEAT_CGROUP_NET 0
#endif

#ifdef ENABLE_PERF_PLUGIN
#define FEAT_PERF 1
#else
#define FEAT_PERF 0
#endif

#ifdef ENABLE_SLABINFO
#define FEAT_SLABINFO 1
#else
#define FEAT_SLABINFO 0
#endif

// Optional Exporters

#ifdef HAVE_KINESIS
#define FEAT_KINESIS 1
#else
#define FEAT_KINESIS 0
#endif

#ifdef ENABLE_EXPORTING_PUBSUB
#define FEAT_PUBSUB 1
#else
#define FEAT_PUBSUB 0
#endif

#ifdef HAVE_MONGOC
#define FEAT_MONGO 1
#else
#define FEAT_MONGO 0
#endif

#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
#define FEAT_REMOTE_WRITE 1
#else
#define FEAT_REMOTE_WRITE 0
#endif

#define FEAT_YES_NO(x) ((x) ? "YES" : "NO")

#ifdef NETDATA_TRACE_ALLOCATIONS
#define FEAT_TRACE_ALLOC 1
#else
#define FEAT_TRACE_ALLOC 0
#endif

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

void print_build_info(void) {
    char *install_type = NULL;
    char *prebuilt_arch = NULL;
    char *prebuilt_distro = NULL;
    get_install_type(&install_type, &prebuilt_arch, &prebuilt_distro);

    printf("Configure options: %s\n", CONFIGURE_COMMAND);

    if (install_type == NULL) {
        printf("Install type: unknown\n");
    } else {
        printf("Install type: %s\n", install_type);
    }

    if (prebuilt_arch != NULL) {
        printf("    Binary architecture: %s\n", prebuilt_arch);
    }

    if (prebuilt_distro != NULL) {
        printf("    Packaging distro: %s\n", prebuilt_distro);
    }

    freez(install_type);
    freez(prebuilt_arch);
    freez(prebuilt_distro);

    printf("Features:\n");
    printf("    dbengine:                   %s\n", FEAT_YES_NO(FEAT_DBENGINE));
    printf("    Native HTTPS:               %s\n", FEAT_YES_NO(FEAT_NATIVE_HTTPS));
    printf("    Netdata Cloud:              %s %s\n", FEAT_YES_NO(FEAT_CLOUD), FEAT_CLOUD_MSG);
    printf("    ACLK:                       %s\n", FEAT_YES_NO(FEAT_CLOUD));
    printf("    TLS Host Verification:      %s\n", FEAT_YES_NO(FEAT_TLS_HOST_VERIFY));
    printf("    Machine Learning:           %s\n", FEAT_YES_NO(FEAT_ML));
    printf("    Stream Compression:         %s\n", FEAT_YES_NO(FEAT_STREAM_COMPRESSION));
    printf("    HTTPD (h2o):                %s\n", FEAT_YES_NO(FEAT_HTTPD));

    printf("Libraries:\n");
    printf("    protobuf:                %s%s\n", FEAT_YES_NO(FEAT_PROTOBUF), FEAT_PROTOBUF_BUNDLED);
    printf("    jemalloc:                %s\n", FEAT_YES_NO(FEAT_JEMALLOC));
    printf("    JSON-C:                  %s\n", FEAT_YES_NO(FEAT_JSONC));
    printf("    libcap:                  %s\n", FEAT_YES_NO(FEAT_LIBCAP));
    printf("    libcrypto:               %s\n", FEAT_YES_NO(FEAT_CRYPTO));
    printf("    libm:                    %s\n", FEAT_YES_NO(FEAT_LIBM));
    printf("    tcalloc:                 %s\n", FEAT_YES_NO(FEAT_TCMALLOC));
    printf("    zlib:                    %s\n", FEAT_YES_NO(1));

    printf("Plugins:\n");
    printf("    apps:                    %s\n", FEAT_YES_NO(FEAT_APPS_PLUGIN));
    printf("    cgroup Network Tracking: %s\n", FEAT_YES_NO(FEAT_CGROUP_NET));
    printf("    CUPS:                    %s\n", FEAT_YES_NO(FEAT_CUPS));
    printf("    debugfs:                 %s\n", FEAT_YES_NO(FEAT_DEBUGFS_PLUGIN));
    printf("    EBPF:                    %s\n", FEAT_YES_NO(FEAT_EBPF));
    printf("    IPMI:                    %s\n", FEAT_YES_NO(FEAT_IPMI));
    printf("    NFACCT:                  %s\n", FEAT_YES_NO(FEAT_NFACCT));
    printf("    perf:                    %s\n", FEAT_YES_NO(FEAT_PERF));
    printf("    slabinfo:                %s\n", FEAT_YES_NO(FEAT_SLABINFO));
    printf("    Xen:                     %s\n", FEAT_YES_NO(FEAT_XEN));
    printf("    Xen VBD Error Tracking:  %s\n", FEAT_YES_NO(FEAT_XEN_VBD_ERROR));

    printf("Exporters:\n");
    printf("    AWS Kinesis:             %s\n", FEAT_YES_NO(FEAT_KINESIS));
    printf("    GCP PubSub:              %s\n", FEAT_YES_NO(FEAT_PUBSUB));
    printf("    MongoDB:                 %s\n", FEAT_YES_NO(FEAT_MONGO));
    printf("    Prometheus Remote Write: %s\n", FEAT_YES_NO(FEAT_REMOTE_WRITE));

    printf("Debug/Developer Features:\n");
    printf("    Trace Allocations:       %s\n", FEAT_YES_NO(FEAT_TRACE_ALLOC));
};

#define FEAT_JSON_BOOL(x) ((x) ? "true" : "false")
// This intentionally does not use JSON-C so it works even if JSON-C is not present
// This is used for anonymous statistics reporting, so it intentionally
// does not include the configure options, which would be very easy to use
// for tracking custom builds (and complicate outputting valid JSON).
void print_build_info_json(void) {
    printf("{\n");
    printf("  \"features\": {\n");
    printf("    \"dbengine\": %s,\n",         FEAT_JSON_BOOL(FEAT_DBENGINE));
    printf("    \"native-https\": %s,\n",     FEAT_JSON_BOOL(FEAT_NATIVE_HTTPS));
    printf("    \"cloud\": %s,\n",            FEAT_JSON_BOOL(FEAT_CLOUD));
#ifdef DISABLE_CLOUD
    printf("    \"cloud-disabled\": true,\n");
#else
    printf("    \"cloud-disabled\": false,\n");
#endif
    printf("    \"aclk\": %s,\n", FEAT_JSON_BOOL(FEAT_CLOUD));

    printf("    \"tls-host-verify\": %s,\n",   FEAT_JSON_BOOL(FEAT_TLS_HOST_VERIFY));
    printf("    \"machine-learning\": %s\n",   FEAT_JSON_BOOL(FEAT_ML));
    printf("    \"stream-compression\": %s\n", FEAT_JSON_BOOL(FEAT_STREAM_COMPRESSION));
    printf("    \"httpd-h2o\": %s\n",          FEAT_JSON_BOOL(FEAT_HTTPD));
    printf("  },\n");

    printf("  \"libs\": {\n");
    printf("    \"protobuf\": %s,\n",         FEAT_JSON_BOOL(FEAT_PROTOBUF));
    printf("    \"protobuf-source\": \"%s\",\n", FEAT_PROTOBUF_BUNDLED);
    printf("    \"jemalloc\": %s,\n",         FEAT_JSON_BOOL(FEAT_JEMALLOC));
    printf("    \"jsonc\": %s,\n",            FEAT_JSON_BOOL(FEAT_JSONC));
    printf("    \"libcap\": %s,\n",           FEAT_JSON_BOOL(FEAT_LIBCAP));
    printf("    \"libcrypto\": %s,\n",        FEAT_JSON_BOOL(FEAT_CRYPTO));
    printf("    \"libm\": %s,\n",             FEAT_JSON_BOOL(FEAT_LIBM));
    printf("    \"tcmalloc\": %s,\n",         FEAT_JSON_BOOL(FEAT_TCMALLOC));
    printf("    \"zlib\": %s\n",              FEAT_JSON_BOOL(1));
    printf("  },\n");

    printf("  \"plugins\": {\n");
    printf("    \"apps\": %s,\n",             FEAT_JSON_BOOL(FEAT_APPS_PLUGIN));
    printf("    \"cgroup-net\": %s,\n",       FEAT_JSON_BOOL(FEAT_CGROUP_NET));
    printf("    \"cups\": %s,\n",             FEAT_JSON_BOOL(FEAT_CUPS));
    printf("    \"debugfs\": %s,\n",          FEAT_JSON_BOOL(FEAT_DEBUGFS_PLUGIN));
    printf("    \"ebpf\": %s,\n",             FEAT_JSON_BOOL(FEAT_EBPF));
    printf("    \"ipmi\": %s,\n",             FEAT_JSON_BOOL(FEAT_IPMI));
    printf("    \"nfacct\": %s,\n",           FEAT_JSON_BOOL(FEAT_NFACCT));
    printf("    \"perf\": %s,\n",             FEAT_JSON_BOOL(FEAT_PERF));
    printf("    \"slabinfo\": %s,\n",         FEAT_JSON_BOOL(FEAT_SLABINFO));
    printf("    \"xen\": %s,\n",              FEAT_JSON_BOOL(FEAT_XEN));
    printf("    \"xen-vbd-error\": %s\n",     FEAT_JSON_BOOL(FEAT_XEN_VBD_ERROR));
    printf("  },\n");

    printf("  \"exporters\": {\n");
    printf("    \"kinesis\": %s,\n",          FEAT_JSON_BOOL(FEAT_KINESIS));
    printf("    \"pubsub\": %s,\n",           FEAT_JSON_BOOL(FEAT_PUBSUB));
    printf("    \"mongodb\": %s,\n",          FEAT_JSON_BOOL(FEAT_MONGO));
    printf("    \"prom-remote-write\": %s\n", FEAT_JSON_BOOL(FEAT_REMOTE_WRITE));
    printf("  }\n");
    printf("  \"debug-n-devel\": {\n");
    printf("    \"trace-allocations\": %s\n  }\n",FEAT_JSON_BOOL(FEAT_TRACE_ALLOC));
    printf("}\n");
};

#define add_to_bi(buffer, str)       \
    { if(first) {                    \
        buffer_strcat (b, str);      \
        first = 0;                   \
    } else                           \
        buffer_strcat (b, "|" str); }

void analytics_build_info(BUFFER *b) {
    int first = 1;
#ifdef ENABLE_DBENGINE
    add_to_bi(b, "dbengine");
#endif
#ifdef ENABLE_HTTPS
    add_to_bi(b, "Native HTTPS");
#endif
#ifdef ENABLE_ACLK
    add_to_bi(b, "Netdata Cloud");
#endif
#if (FEAT_TLS_HOST_VERIFY!=0)
    add_to_bi(b, "TLS Host Verification");
#endif
#ifdef ENABLE_ML
    add_to_bi(b, "Machine Learning");
#endif
#ifdef ENABLE_COMPRESSION
    add_to_bi(b, "Stream Compression");
#endif

#ifdef HAVE_PROTOBUF
    add_to_bi(b, "protobuf");
#endif
#ifdef ENABLE_JEMALLOC
    add_to_bi(b, "jemalloc");
#endif
#ifdef ENABLE_JSONC
    add_to_bi(b, "JSON-C");
#endif
#ifdef HAVE_CAPABILITY
    add_to_bi(b, "libcap");
#endif
#ifdef HAVE_CRYPTO
    add_to_bi(b, "libcrypto");
#endif
#ifdef STORAGE_WITH_MATH
    add_to_bi(b, "libm");
#endif

#ifdef ENABLE_TCMALLOC
    add_to_bi(b, "tcalloc");
#endif
    add_to_bi(b, "zlib");

#ifdef ENABLE_APPS_PLUGIN
    add_to_bi(b, "apps");
#endif
#ifdef ENABLE_DEBUGFS_PLUGIN
    add_to_bi(b, "debugfs");
#endif
#ifdef HAVE_SETNS
    add_to_bi(b, "cgroup Network Tracking");
#endif
#ifdef HAVE_CUPS
    add_to_bi(b, "CUPS");
#endif
#ifdef HAVE_LIBBPF
    add_to_bi(b, "EBPF");
#endif
#ifdef HAVE_FREEIPMI
    add_to_bi(b, "IPMI");
#endif
#ifdef HAVE_NFACCT
    add_to_bi(b, "NFACCT");
#endif
#ifdef ENABLE_PERF_PLUGIN
    add_to_bi(b, "perf");
#endif
#ifdef ENABLE_SLABINFO
    add_to_bi(b, "slabinfo");
#endif
#ifdef HAVE_LIBXENSTAT
    add_to_bi(b, "Xen");
#endif
#ifdef HAVE_XENSTAT_VBD_ERROR
    add_to_bi(b, "Xen VBD Error Tracking");
#endif

#ifdef HAVE_KINESIS
    add_to_bi(b, "AWS Kinesis");
#endif
#ifdef ENABLE_EXPORTING_PUBSUB
    add_to_bi(b, "GCP PubSub");
#endif
#ifdef HAVE_MONGOC
    add_to_bi(b, "MongoDB");
#endif
#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
    add_to_bi(b, "Prometheus Remote Write");
#endif
#ifdef NETDATA_TRACE_ALLOCATIONS
    add_to_bi(b, "DebugTraceAlloc");
#endif
}
