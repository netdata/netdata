// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include "./config.h"

// Optional features

#ifdef ENABLE_ACLK
#define FEAT_CLOUD 1
#define FEAT_CLOUD_MSG ""
#ifdef ACLK_NG
#define ACLK_IMPL "Next Generation"
#else
#define ACLK_IMPL "Legacy"
#endif
#else
#define ACLK_IMPL ""
#ifdef DISABLE_CLOUD
#define FEAT_CLOUD 0
#define FEAT_CLOUD_MSG "(by user request)"
#else
#define FEAT_CLOUD 0
#define FEAT_CLOUD_MSG ""
#endif
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

// Optional libraries

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

#ifndef ACLK_NG
#ifdef ACLK_NO_LIBMOSQ
#define FEAT_MOSQUITTO 0
#else
#define FEAT_MOSQUITTO 1
#endif

#ifdef ACLK_NO_LWS
#define FEAT_LWS 0
#define FEAT_LWS_MSG ""
#else
#ifdef ENABLE_ACLK
#include <libwebsockets.h>
#endif
#ifdef BUNDLED_LWS
#define FEAT_LWS 1
#define FEAT_LWS_MSG "static"
#else
#define FEAT_LWS 1
#define FEAT_LWS_MSG "shared-lib"
#endif
#endif
#endif /* ACLK_NG */

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

void print_build_info(void) {
    printf("Configure options: %s\n", CONFIGURE_COMMAND);

    printf("Features:\n");
    printf("    dbengine:                %s\n", FEAT_YES_NO(FEAT_DBENGINE));
    printf("    Native HTTPS:            %s\n", FEAT_YES_NO(FEAT_NATIVE_HTTPS));
    printf("    Netdata Cloud:           %s %s\n", FEAT_YES_NO(FEAT_CLOUD), FEAT_CLOUD_MSG);
#if FEAT_CLOUD == 1
    printf("    Cloud Implementation:    %s\n", ACLK_IMPL);
#endif
    printf("    TLS Host Verification:   %s\n", FEAT_YES_NO(FEAT_TLS_HOST_VERIFY));

    printf("Libraries:\n");
    printf("    jemalloc:                %s\n", FEAT_YES_NO(FEAT_JEMALLOC));
    printf("    JSON-C:                  %s\n", FEAT_YES_NO(FEAT_JSONC));
    printf("    libcap:                  %s\n", FEAT_YES_NO(FEAT_LIBCAP));
    printf("    libcrypto:               %s\n", FEAT_YES_NO(FEAT_CRYPTO));
    printf("    libm:                    %s\n", FEAT_YES_NO(FEAT_LIBM));
#ifndef ACLK_NG
#if defined(ENABLE_ACLK)
    printf("    LWS:                     %s %s v%d.%d.%d\n", FEAT_YES_NO(FEAT_LWS), FEAT_LWS_MSG, LWS_LIBRARY_VERSION_MAJOR, LWS_LIBRARY_VERSION_MINOR, LWS_LIBRARY_VERSION_PATCH);
#else
    printf("    LWS:                     %s %s\n", FEAT_YES_NO(FEAT_LWS), FEAT_LWS_MSG);
#endif
    printf("    mosquitto:               %s\n", FEAT_YES_NO(FEAT_MOSQUITTO));
#endif
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
#if FEAT_CLOUD == 1
    printf("    \"cloud-implementation\": \"%s\",\n", ACLK_IMPL);
#endif
    printf("    \"tls-host-verify\": %s\n",   FEAT_JSON_BOOL(FEAT_TLS_HOST_VERIFY));
    printf("  },\n");

    printf("  \"libs\": {\n");
    printf("    \"jemalloc\": %s,\n",         FEAT_JSON_BOOL(FEAT_JEMALLOC));
    printf("    \"jsonc\": %s,\n",            FEAT_JSON_BOOL(FEAT_JSONC));
    printf("    \"libcap\": %s,\n",           FEAT_JSON_BOOL(FEAT_LIBCAP));
    printf("    \"libcrypto\": %s,\n",        FEAT_JSON_BOOL(FEAT_CRYPTO));
    printf("    \"libm\": %s,\n",             FEAT_JSON_BOOL(FEAT_LIBM));
#ifndef ACLK_NG
#if defined(ENABLE_ACLK)
    printf("    \"lws\": %s,\n", FEAT_JSON_BOOL(FEAT_LWS));
    printf("    \"lws-version\": \"%d.%d.%d\",\n", LWS_LIBRARY_VERSION_MAJOR, LWS_LIBRARY_VERSION_MINOR, LWS_LIBRARY_VERSION_PATCH);
    printf("    \"lws-type\": \"%s\",\n", FEAT_LWS_MSG);
#else
    printf("    \"lws\": %s,\n",              FEAT_JSON_BOOL(FEAT_LWS));
#endif
    printf("    \"mosquitto\": %s,\n",        FEAT_JSON_BOOL(FEAT_MOSQUITTO));
#endif
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
