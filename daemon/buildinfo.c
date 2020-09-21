// SPDX-License-Identifier: GPL-3.0-or-later

#include <stdio.h>
#include "./config.h"

// Optional features

#ifdef ENABLE_ACLK
#define FEAT_CLOUD "YES"
#else
#ifdef DISABLE_CLOUD
#define FEAT_CLOUD "NO (by user request e.g. '--disable-cloud')"
#else
#define FEAT_CLOUD "NO"
#endif
#endif

#ifdef ENABLE_DBENGINE
#define FEAT_DBENGINE "YES"
#else
#define FEAT_DBENGINE "NO"
#endif

#if defined(HAVE_X509_VERIFY_PARAM_set1_host) && HAVE_X509_VERIFY_PARAM_set1_host == 1
#define FEAT_TLS_HOST_VERIFY "YES"
#else
#define FEAT_TLS_HOST_VERIFY "NO"
#endif

#ifdef ENABLE_HTTPS
#define FEAT_NATIVE_HTTPS "YES"
#else
#define FEAT_NATIVE_HTTPS "NO"
#endif

// Optional libraries

#ifdef ENABLE_JSONC
#define FEAT_JSONC "YES"
#else
#define FEAT_JSONC "NO"
#endif

#ifdef ENABLE_JEMALLOC
#define FEAT_JEMALLOC "YES"
#else
#define FEAT_JEMALLOC "NO"
#endif

#ifdef ENABLE_TCMALLOC
#define FEAT_TCMALLOC "YES"
#else
#define FEAT_TCMALLOC "NO"
#endif

#ifdef HAVE_CAPABILITY
#define FEAT_LIBCAP "YES"
#else
#define FEAT_LIBCAP "NO"
#endif

#ifdef ACLK_NO_LIBMOSQ
#define FEAT_MOSQUITTO "NO"
#else
#define FEAT_MOSQUITTO "YES"
#endif

#ifdef ACLK_NO_LWS
#define FEAT_LWS "NO"
#else
#define FEAT_LWS "YES"
#endif

#ifdef NETDATA_WITH_ZLIB
#define FEAT_ZLIB "YES"
#else
#define FEAT_ZLIB "NO"
#endif

#ifdef STORAGE_WITH_MATH
#define FEAT_LIBM "YES"
#else
#define FEAT_LIBM "NO"
#endif

#ifdef HAVE_CRYPTO
#define FEAT_CRYPTO "YES"
#else
#define FEAT_CRYPTO "NO"
#endif

// Optional plugins

#ifdef ENABLE_APPS_PLUGIN
#define FEAT_APPS_PLUGIN "YES"
#else
#define FEAT_APPS_PLUGIN "NO"
#endif

#ifdef HAVE_FREEIPMI
#define FEAT_IPMI "YES"
#else
#define FEAT_IPMI "NO"
#endif

#ifdef HAVE_CUPS
#define FEAT_CUPS "YES"
#else
#define FEAT_CUPS "NO"
#endif

#ifdef HAVE_LIBMNL
#define FEAT_NFACCT "YES"
#else
#define FEAT_NFACCT "NO"
#endif

#ifdef HAVE_LIBXENSTAT
#define FEAT_XEN "YES"
#else
#define FEAT_XEN "NO"
#endif

#ifdef HAVE_XENSTAT_VBD_ERROR
#define FEAT_XEN_VBD_ERROR "YES"
#else
#define FEAT_XEN_VBD_ERROR "NO"
#endif

#ifdef HAVE_LIBBPF
#define FEAT_EBPF "YES"
#else
#define FEAT_EBPF "NO"
#endif

#ifdef HAVE_SETNS
#define FEAT_CGROUP_NET "YES"
#else
#define FEAT_CGROUP_NET "NO"
#endif

#ifdef ENABLE_PERF_PLUGIN
#define FEAT_PERF "YES"
#else
#define FEAT_PERF "NO"
#endif

#ifdef ENABLE_SLABINFO
#define FEAT_SLABINFO "YES"
#else
#define FEAT_SLABINFO "NO"
#endif

// Optional Exporters

#ifdef HAVE_KINESIS
#define FEAT_KINESIS "YES"
#else
#define FEAT_KINESIS "NO"
#endif

#ifdef ENABLE_EXPORTING_PUBSUB
#define FEAT_PUBSUB "YES"
#else
#define FEAT_PUBSUB "NO"
#endif

#ifdef HAVE_MONGOC
#define FEAT_MONGO "YES"
#else
#define FEAT_MONGO "NO"
#endif

#ifdef ENABLE_PROMETHEUS_REMOTE_WRITE
#define FEAT_REMOTE_WRITE "YES"
#else
#define FEAT_REMOTE_WRITE "NO"
#endif


void print_build_info(void) {
    printf("Configure options: %s\n", CONFIGURE_COMMAND);

    printf("Features:\n");
    printf("    dbengine:                %s\n", FEAT_DBENGINE);
    printf("    Native HTTPS:            %s\n", FEAT_NATIVE_HTTPS);
    printf("    Netdata Cloud:           %s\n", FEAT_CLOUD);
    printf("    TLS Host Verification:   %s\n", FEAT_TLS_HOST_VERIFY);

    printf("Libraries:\n");
    printf("    jemalloc:                %s\n", FEAT_JEMALLOC);
    printf("    JSON-C:                  %s\n", FEAT_JSONC);
    printf("    libcap:                  %s\n", FEAT_LIBCAP);
    printf("    libcrypto:               %s\n", FEAT_CRYPTO);
    printf("    libm:                    %s\n", FEAT_LIBM);
    printf("    LWS:                     %s\n", FEAT_LWS);
    printf("    mosquitto:               %s\n", FEAT_MOSQUITTO);
    printf("    tcalloc:                 %s\n", FEAT_TCMALLOC);
    printf("    zlib:                    %s\n", FEAT_ZLIB);

    printf("Plugins:\n");
    printf("    apps:                    %s\n", FEAT_APPS_PLUGIN);
    printf("    cgroup Network Tracking: %s\n", FEAT_CGROUP_NET);
    printf("    CUPS:                    %s\n", FEAT_CUPS);
    printf("    EBPF:                    %s\n", FEAT_EBPF);
    printf("    IPMI:                    %s\n", FEAT_IPMI);
    printf("    NFACCT:                  %s\n", FEAT_NFACCT);
    printf("    perf:                    %s\n", FEAT_PERF);
    printf("    slabinfo:                %s\n", FEAT_SLABINFO);
    printf("    Xen:                     %s\n", FEAT_XEN);
    printf("    Xen VBD Error Tracking:  %s\n", FEAT_XEN_VBD_ERROR);

    printf("Exporters:\n");
    printf("    AWS Kinesis:             %s\n", FEAT_KINESIS);
    printf("    GCP PubSub:              %s\n", FEAT_PUBSUB);
    printf("    MongoDB:                 %s\n", FEAT_MONGO);
    printf("    Prometheus Remote Write: %s\n", FEAT_REMOTE_WRITE);
};
