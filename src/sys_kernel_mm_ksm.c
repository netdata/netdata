#include "common.h"

typedef struct ksm_name_value {
    char filename[FILENAME_MAX + 1];
    unsigned long long value;
} KSM_NAME_VALUE;

#define PAGES_SHARED 0
#define PAGES_SHARING 1
#define PAGES_UNSHARED 2
#define PAGES_VOLATILE 3
#define PAGES_TO_SCAN 4

KSM_NAME_VALUE values[] = {
        [PAGES_SHARED] = { "/sys/kernel/mm/ksm/pages_shared", 0ULL },
        [PAGES_SHARING] = { "/sys/kernel/mm/ksm/pages_sharing", 0ULL },
        [PAGES_UNSHARED] = { "/sys/kernel/mm/ksm/pages_unshared", 0ULL },
        [PAGES_VOLATILE] = { "/sys/kernel/mm/ksm/pages_volatile", 0ULL },
        [PAGES_TO_SCAN] = { "/sys/kernel/mm/ksm/pages_to_scan", 0ULL },
};

int do_sys_kernel_mm_ksm(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff_pages_shared = NULL, *ff_pages_sharing = NULL, *ff_pages_unshared = NULL, *ff_pages_volatile = NULL, *ff_pages_to_scan = NULL;
    static long page_size = -1;

    if(page_size == -1)
        page_size = sysconf(_SC_PAGESIZE);

    if(!ff_pages_shared) {
        snprintfz(values[PAGES_SHARED].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_shared");
        snprintfz(values[PAGES_SHARED].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_shared", values[PAGES_SHARED].filename));
        ff_pages_shared = procfile_open(values[PAGES_SHARED].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(!ff_pages_sharing) {
        snprintfz(values[PAGES_SHARING].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_sharing");
        snprintfz(values[PAGES_SHARING].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_sharing", values[PAGES_SHARING].filename));
        ff_pages_sharing = procfile_open(values[PAGES_SHARING].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(!ff_pages_unshared) {
        snprintfz(values[PAGES_UNSHARED].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_unshared");
        snprintfz(values[PAGES_UNSHARED].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_unshared", values[PAGES_UNSHARED].filename));
        ff_pages_unshared = procfile_open(values[PAGES_UNSHARED].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(!ff_pages_volatile) {
        snprintfz(values[PAGES_VOLATILE].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_volatile");
        snprintfz(values[PAGES_VOLATILE].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_volatile", values[PAGES_VOLATILE].filename));
        ff_pages_volatile = procfile_open(values[PAGES_VOLATILE].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(!ff_pages_to_scan) {
        snprintfz(values[PAGES_TO_SCAN].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_to_scan");
        snprintfz(values[PAGES_TO_SCAN].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_to_scan", values[PAGES_TO_SCAN].filename));
        ff_pages_to_scan = procfile_open(values[PAGES_TO_SCAN].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(!ff_pages_shared || !ff_pages_sharing || !ff_pages_unshared || !ff_pages_volatile || !ff_pages_to_scan) return 1;

    unsigned long long pages_shared = 0, pages_sharing = 0, pages_unshared = 0, pages_volatile = 0, pages_to_scan = 0, offered = 0, saved = 0;

    ff_pages_shared = procfile_readall(ff_pages_shared);
    if(!ff_pages_shared) return 0; // we return 0, so that we will retry to open it next time
    pages_shared = str2ull(procfile_lineword(ff_pages_shared, 0, 0));

    ff_pages_sharing = procfile_readall(ff_pages_sharing);
    if(!ff_pages_sharing) return 0; // we return 0, so that we will retry to open it next time
    pages_sharing = str2ull(procfile_lineword(ff_pages_sharing, 0, 0));

    ff_pages_unshared = procfile_readall(ff_pages_unshared);
    if(!ff_pages_unshared) return 0; // we return 0, so that we will retry to open it next time
    pages_unshared = str2ull(procfile_lineword(ff_pages_unshared, 0, 0));

    ff_pages_volatile = procfile_readall(ff_pages_volatile);
    if(!ff_pages_volatile) return 0; // we return 0, so that we will retry to open it next time
    pages_volatile = str2ull(procfile_lineword(ff_pages_volatile, 0, 0));

    ff_pages_to_scan = procfile_readall(ff_pages_to_scan);
    if(!ff_pages_to_scan) return 0; // we return 0, so that we will retry to open it next time
    pages_to_scan = str2ull(procfile_lineword(ff_pages_to_scan, 0, 0));

    offered = pages_sharing + pages_shared + pages_unshared + pages_volatile;
    saved = pages_sharing - pages_shared;

    if(!offered || !pages_to_scan) return 0;

    RRDSET *st;

    // --------------------------------------------------------------------

    st = rrdset_find_localhost("mem.ksm");
    if(!st) {
        st = rrdset_create_localhost("mem", "ksm", NULL, "ksm", NULL, "Kernel Same Page Merging", "MB", 5000
                                     , update_every, RRDSET_TYPE_AREA);

        rrddim_add(st, "shared", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(st, "unshared", NULL, -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(st, "sharing", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(st, "volatile", NULL, -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(st, "to_scan", "to scan", -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(st);

    rrddim_set(st, "shared", pages_shared * page_size);
    rrddim_set(st, "unshared", pages_unshared * page_size);
    rrddim_set(st, "sharing", pages_sharing * page_size);
    rrddim_set(st, "volatile", pages_volatile * page_size);
    rrddim_set(st, "to_scan", pages_to_scan * page_size);
    rrdset_done(st);

    st = rrdset_find_localhost("mem.ksm_savings");
    if(!st) {
        st = rrdset_create_localhost("mem", "ksm_savings", NULL, "ksm", NULL, "Kernel Same Page Merging Savings", "MB"
                                     , 5001, update_every, RRDSET_TYPE_AREA);

        rrddim_add(st, "savings", NULL, -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        rrddim_add(st, "offered", NULL, 1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(st);

    rrddim_set(st, "savings", saved * page_size);
    rrddim_set(st, "offered", offered * page_size);
    rrdset_done(st);

    st = rrdset_find_localhost("mem.ksm_ratios");
    if(!st) {
        st = rrdset_create_localhost("mem", "ksm_ratios", NULL, "ksm", NULL, "Kernel Same Page Merging Effectiveness"
                                     , "percentage", 5002, update_every, RRDSET_TYPE_LINE);

        rrddim_add(st, "savings", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
    }
    else rrdset_next(st);

    rrddim_set(st, "savings", (saved * 1000000) / offered);
    rrdset_done(st);

    return 0;
}
