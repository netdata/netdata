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

#ifdef ENABLE_NEW_CLOUD_PROTOCOL
#define NEW_CLOUD_PROTO 1
#else
#define NEW_CLOUD_PROTO 0
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

#ifdef NETDATA_WITH_ZLIB
#define FEAT_ZLIB 1
#else
#define FEAT_ZLIB 0
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

struct install_type_info get_install_type() {
    char *install_type_filename;
    struct install_type_info ret = {.install_type = NULL, .prebuilt_arch = NULL, .prebuilt_distro = NULL};

    int install_type_filename_len = (strlen(netdata_configured_user_config_dir) + strlen(".install-type") + 3);
    install_type_filename = mallocz(sizeof(char) * install_type_filename_len);
    snprintfz(install_type_filename, install_type_filename_len - 1, "%s/%s", netdata_configured_user_config_dir, ".install-type");

    FILE *fp = fopen(install_type_filename, "r");
    if (fp) {
        char *s, buf[256 + 1];
        size_t len = 0;

        while ((s = fgets_trim_len(buf, 256, fp, &len))) {
            if (!strncmp(buf, "INSTALL_TYPE='", 14))
                ret.install_type = strdupz((char *)get_value_from_key(buf, "INSTALL_TYPE"));
            else if (!strncmp(buf, "PREBUILT_ARCH='", 15))
                ret.prebuilt_arch = strdupz((char *)get_value_from_key(buf, "PREBUILT_ARCH"));
            else if (!strncmp(buf, "PREBUILT_DISTRO='", 17))
                ret.prebuilt_distro = strdupz((char *)get_value_from_key(buf, "PREBUILT_DISTRO"));
        }
        fclose(fp);
    }
    freez(install_type_filename);

    return ret;
}

void print_build_info(void) {
    struct install_type_info t = get_install_type();

    printf("Configure options: %s\n", CONFIGURE_COMMAND);

    if (t.install_type == NULL) {
        printf("Install type: unknown\n");
    } else {
        printf("Install type: %s\n", t.install_type);
    }

    if (t.prebuilt_arch != NULL) {
        printf("    Binary architecture: %s\n", t.prebuilt_arch);
    }

    if (t.prebuilt_distro != NULL) {
        printf("    Packaging distro: %s\n", t.prebuilt_distro);
    }

    freez(t.install_type);
    freez(t.prebuilt_arch);
    freez(t.prebuilt_distro);

    printf("Features:\n");
    printf("    dbengine:                   %s\n", FEAT_YES_NO(FEAT_DBENGINE));
    printf("    Native HTTPS:               %s\n", FEAT_YES_NO(FEAT_NATIVE_HTTPS));
    printf("    Netdata Cloud:              %s %s\n", FEAT_YES_NO(FEAT_CLOUD), FEAT_CLOUD_MSG);
    printf("    ACLK Next Generation:       %s\n", FEAT_YES_NO(FEAT_CLOUD));
    printf("    ACLK-NG New Cloud Protocol: %s\n", FEAT_YES_NO(NEW_CLOUD_PROTO));
    printf("    ACLK Legacy:                %s\n", FEAT_YES_NO(0));
    printf("    TLS Host Verification:      %s\n", FEAT_YES_NO(FEAT_TLS_HOST_VERIFY));
    printf("    Machine Learning:           %s\n", FEAT_YES_NO(FEAT_ML));
    printf("    Stream Compression:         %s\n", FEAT_YES_NO(FEAT_STREAM_COMPRESSION));

    printf("Libraries:\n");
    printf("    protobuf:                %s%s\n", FEAT_YES_NO(FEAT_PROTOBUF), FEAT_PROTOBUF_BUNDLED);
    printf("    jemalloc:                %s\n", FEAT_YES_NO(FEAT_JEMALLOC));
    printf("    JSON-C:                  %s\n", FEAT_YES_NO(FEAT_JSONC));
    printf("    libcap:                  %s\n", FEAT_YES_NO(FEAT_LIBCAP));
    printf("    libcrypto:               %s\n", FEAT_YES_NO(FEAT_CRYPTO));
    printf("    libm:                    %s\n", FEAT_YES_NO(FEAT_LIBM));
    printf("    tcalloc:                 %s\n", FEAT_YES_NO(FEAT_TCMALLOC));
    printf("    zlib:                    %s\n", FEAT_YES_NO(FEAT_ZLIB));

    printf("Plugins:\n");
    printf("    apps:                    %s\n", FEAT_YES_NO(FEAT_APPS_PLUGIN));
    printf("    cgroup Network Tracking: %s\n", FEAT_YES_NO(FEAT_CGROUP_NET));
    printf("    CUPS:                    %s\n", FEAT_YES_NO(FEAT_CUPS));
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
    printf("    \"aclk-ng\": %s,\n", FEAT_JSON_BOOL(FEAT_CLOUD));
    printf("    \"aclk-ng-new-cloud-proto\": %s,\n", FEAT_JSON_BOOL(NEW_CLOUD_PROTO));
    printf("    \"aclk-legacy\": %s,\n", FEAT_JSON_BOOL(0));

    printf("    \"tls-host-verify\": %s,\n",   FEAT_JSON_BOOL(FEAT_TLS_HOST_VERIFY));
    printf("    \"machine-learning\": %s\n",   FEAT_JSON_BOOL(FEAT_ML));
    printf("    \"stream-compression\": %s\n",   FEAT_JSON_BOOL(FEAT_STREAM_COMPRESSION));
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
    printf("    \"zlib\": %s\n",              FEAT_JSON_BOOL(FEAT_ZLIB));
    printf("  },\n");

    printf("  \"plugins\": {\n");
    printf("    \"apps\": %s,\n",             FEAT_JSON_BOOL(FEAT_APPS_PLUGIN));
    printf("    \"cgroup-net\": %s,\n",       FEAT_JSON_BOOL(FEAT_CGROUP_NET));
    printf("    \"cups\": %s,\n",             FEAT_JSON_BOOL(FEAT_CUPS));
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
    printf("}\n");
};

//return a list of enabled features for use in analytics
//find a way to have proper |
void analytics_build_info(BUFFER *b) {
    if(FEAT_DBENGINE)        buffer_strcat (b, "dbengine");
    if(FEAT_NATIVE_HTTPS)    buffer_strcat (b, "|Native HTTPS");
    if(FEAT_CLOUD)           buffer_strcat (b, "|Netdata Cloud");
    if(FEAT_CLOUD)           buffer_strcat (b, "|ACLK Next Generation");
    if(NEW_CLOUD_PROTO)      buffer_strcat (b, "|New Cloud Protocol Support");
    if(FEAT_TLS_HOST_VERIFY) buffer_strcat (b, "|TLS Host Verification");
    if(FEAT_ML)              buffer_strcat (b, "|Machine Learning");
    if(FEAT_STREAM_COMPRESSION) buffer_strcat (b, "|Stream Compression");

    if(FEAT_PROTOBUF)        buffer_strcat (b, "|protobuf");
    if(FEAT_JEMALLOC)        buffer_strcat (b, "|jemalloc");
    if(FEAT_JSONC)           buffer_strcat (b, "|JSON-C");
    if(FEAT_LIBCAP)          buffer_strcat (b, "|libcap");
    if(FEAT_CRYPTO)          buffer_strcat (b, "|libcrypto");
    if(FEAT_LIBM)            buffer_strcat (b, "|libm");

    if(FEAT_TCMALLOC)       buffer_strcat(b, "|tcalloc");
    if(FEAT_ZLIB)           buffer_strcat(b, "|zlib");

    if(FEAT_APPS_PLUGIN)    buffer_strcat(b, "|apps");
    if(FEAT_CGROUP_NET)     buffer_strcat(b, "|cgroup Network Tracking");
    if(FEAT_CUPS)           buffer_strcat(b, "|CUPS");
    if(FEAT_EBPF)           buffer_strcat(b, "|EBPF");
    if(FEAT_IPMI)           buffer_strcat(b, "|IPMI");
    if(FEAT_NFACCT)         buffer_strcat(b, "|NFACCT");
    if(FEAT_PERF)           buffer_strcat(b, "|perf");
    if(FEAT_SLABINFO)       buffer_strcat(b, "|slabinfo");
    if(FEAT_XEN)            buffer_strcat(b, "|Xen");
    if(FEAT_XEN_VBD_ERROR)  buffer_strcat(b, "|Xen VBD Error Tracking");

    if(FEAT_KINESIS)        buffer_strcat(b, "|AWS Kinesis");
    if(FEAT_PUBSUB)         buffer_strcat(b, "|GCP PubSub");
    if(FEAT_MONGO)          buffer_strcat(b, "|MongoDB");
    if(FEAT_REMOTE_WRITE)   buffer_strcat(b, "|Prometheus Remote Write");
}
