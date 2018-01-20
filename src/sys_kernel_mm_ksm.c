#include "common.h"

typedef struct ksm_name_value {
    char filename[FILENAME_MAX + 1];
    unsigned long long value;
} KSM_NAME_VALUE;

#define PAGES_SHARED   0
#define PAGES_SHARING  1
#define PAGES_UNSHARED 2
#define PAGES_VOLATILE 3
#define PAGES_TO_SCAN  4

KSM_NAME_VALUE values[] = {
        [PAGES_SHARED]   = { "/sys/kernel/mm/ksm/pages_shared",   0ULL },
        [PAGES_SHARING]  = { "/sys/kernel/mm/ksm/pages_sharing",  0ULL },
        [PAGES_UNSHARED] = { "/sys/kernel/mm/ksm/pages_unshared", 0ULL },
        [PAGES_VOLATILE] = { "/sys/kernel/mm/ksm/pages_volatile", 0ULL },
        [PAGES_TO_SCAN]  = { "/sys/kernel/mm/ksm/pages_to_scan",  0ULL },
};

int do_sys_kernel_mm_ksm(int update_every, usec_t dt) {
    (void)dt;
    static procfile *ff_pages_shared = NULL, *ff_pages_sharing = NULL, *ff_pages_unshared = NULL, *ff_pages_volatile = NULL, *ff_pages_to_scan = NULL;
    static unsigned long page_size = 0;

    if(unlikely(page_size == 0))
        page_size = (unsigned long)sysconf(_SC_PAGESIZE);

    if(unlikely(!ff_pages_shared)) {
        snprintfz(values[PAGES_SHARED].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_shared");
        snprintfz(values[PAGES_SHARED].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_shared", values[PAGES_SHARED].filename));
        ff_pages_shared = procfile_open(values[PAGES_SHARED].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(unlikely(!ff_pages_sharing)) {
        snprintfz(values[PAGES_SHARING].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_sharing");
        snprintfz(values[PAGES_SHARING].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_sharing", values[PAGES_SHARING].filename));
        ff_pages_sharing = procfile_open(values[PAGES_SHARING].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(unlikely(!ff_pages_unshared)) {
        snprintfz(values[PAGES_UNSHARED].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_unshared");
        snprintfz(values[PAGES_UNSHARED].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_unshared", values[PAGES_UNSHARED].filename));
        ff_pages_unshared = procfile_open(values[PAGES_UNSHARED].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(unlikely(!ff_pages_volatile)) {
        snprintfz(values[PAGES_VOLATILE].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_volatile");
        snprintfz(values[PAGES_VOLATILE].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_volatile", values[PAGES_VOLATILE].filename));
        ff_pages_volatile = procfile_open(values[PAGES_VOLATILE].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(unlikely(!ff_pages_to_scan)) {
        snprintfz(values[PAGES_TO_SCAN].filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/sys/kernel/mm/ksm/pages_to_scan");
        snprintfz(values[PAGES_TO_SCAN].filename, FILENAME_MAX, "%s", config_get("plugin:proc:/sys/kernel/mm/ksm", "/sys/kernel/mm/ksm/pages_to_scan", values[PAGES_TO_SCAN].filename));
        ff_pages_to_scan = procfile_open(values[PAGES_TO_SCAN].filename, " \t:", PROCFILE_FLAG_DEFAULT);
    }

    if(unlikely(!ff_pages_shared || !ff_pages_sharing || !ff_pages_unshared || !ff_pages_volatile || !ff_pages_to_scan))
        return 1;

    unsigned long long pages_shared = 0, pages_sharing = 0, pages_unshared = 0, pages_volatile = 0, pages_to_scan = 0, offered = 0, saved = 0;

    ff_pages_shared = procfile_readall(ff_pages_shared);
    if(unlikely(!ff_pages_shared)) return 0; // we return 0, so that we will retry to open it next time
    pages_shared = str2ull(procfile_lineword(ff_pages_shared, 0, 0));

    ff_pages_sharing = procfile_readall(ff_pages_sharing);
    if(unlikely(!ff_pages_sharing)) return 0; // we return 0, so that we will retry to open it next time
    pages_sharing = str2ull(procfile_lineword(ff_pages_sharing, 0, 0));

    ff_pages_unshared = procfile_readall(ff_pages_unshared);
    if(unlikely(!ff_pages_unshared)) return 0; // we return 0, so that we will retry to open it next time
    pages_unshared = str2ull(procfile_lineword(ff_pages_unshared, 0, 0));

    ff_pages_volatile = procfile_readall(ff_pages_volatile);
    if(unlikely(!ff_pages_volatile)) return 0; // we return 0, so that we will retry to open it next time
    pages_volatile = str2ull(procfile_lineword(ff_pages_volatile, 0, 0));

    ff_pages_to_scan = procfile_readall(ff_pages_to_scan);
    if(unlikely(!ff_pages_to_scan)) return 0; // we return 0, so that we will retry to open it next time
    pages_to_scan = str2ull(procfile_lineword(ff_pages_to_scan, 0, 0));

    offered = pages_sharing + pages_shared + pages_unshared + pages_volatile;
    saved = pages_sharing;

    if(unlikely(!offered || !pages_to_scan)) return 0;

    // --------------------------------------------------------------------

    {
        static RRDSET *st_mem_ksm = NULL;
        static RRDDIM *rd_shared = NULL, *rd_unshared = NULL, *rd_sharing = NULL, *rd_volatile = NULL, *rd_to_scan = NULL;

        if (unlikely(!st_mem_ksm)) {
            st_mem_ksm = rrdset_create_localhost(
                    "mem"
                    , "ksm"
                    , NULL
                    , "ksm"
                    , NULL
                    , "Kernel Same Page Merging"
                    , "MB"
                    , "proc"
                    , "/sys/kernel/mm/ksm"
                    , NETDATA_CHART_PRIO_MEM_KSM
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_shared   = rrddim_add(st_mem_ksm, "shared",   NULL,      1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_unshared = rrddim_add(st_mem_ksm, "unshared", NULL,     -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_sharing  = rrddim_add(st_mem_ksm, "sharing",  NULL,      1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_volatile = rrddim_add(st_mem_ksm, "volatile", NULL,     -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_to_scan  = rrddim_add(st_mem_ksm, "to_scan", "to scan", -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_mem_ksm);

        rrddim_set_by_pointer(st_mem_ksm, rd_shared,   pages_shared   * page_size);
        rrddim_set_by_pointer(st_mem_ksm, rd_unshared, pages_unshared * page_size);
        rrddim_set_by_pointer(st_mem_ksm, rd_sharing,  pages_sharing  * page_size);
        rrddim_set_by_pointer(st_mem_ksm, rd_volatile, pages_volatile * page_size);
        rrddim_set_by_pointer(st_mem_ksm, rd_to_scan,  pages_to_scan  * page_size);

        rrdset_done(st_mem_ksm);
    }

    // --------------------------------------------------------------------

    {
        static RRDSET *st_mem_ksm_savings = NULL;
        static RRDDIM *rd_savings = NULL, *rd_offered = NULL;

        if (unlikely(!st_mem_ksm_savings)) {
            st_mem_ksm_savings = rrdset_create_localhost(
                    "mem"
                    , "ksm_savings"
                    , NULL
                    , "ksm"
                    , NULL
                    , "Kernel Same Page Merging Savings"
                    , "MB"
                    , "proc"
                    , "/sys/kernel/mm/ksm"
                    , NETDATA_CHART_PRIO_MEM_KSM + 1
                    , update_every
                    , RRDSET_TYPE_AREA
            );

            rd_savings = rrddim_add(st_mem_ksm_savings, "savings", NULL, -1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
            rd_offered = rrddim_add(st_mem_ksm_savings, "offered", NULL,  1, 1024 * 1024, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_mem_ksm_savings);

        rrddim_set_by_pointer(st_mem_ksm_savings, rd_savings, saved * page_size);
        rrddim_set_by_pointer(st_mem_ksm_savings, rd_offered, offered * page_size);

        rrdset_done(st_mem_ksm_savings);
    }

    // --------------------------------------------------------------------

    {
        static RRDSET *st_mem_ksm_ratios = NULL;
        static RRDDIM *rd_savings = NULL;

        if (unlikely(!st_mem_ksm_ratios)) {
            st_mem_ksm_ratios = rrdset_create_localhost(
                    "mem"
                    , "ksm_ratios"
                    , NULL
                    , "ksm"
                    , NULL
                    , "Kernel Same Page Merging Effectiveness"
                    , "percentage"
                    , "proc"
                    , "/sys/kernel/mm/ksm"
                    , NETDATA_CHART_PRIO_MEM_KSM + 2
                    , update_every
                    , RRDSET_TYPE_LINE
            );

            rd_savings = rrddim_add(st_mem_ksm_ratios, "savings", NULL, 1, 10000, RRD_ALGORITHM_ABSOLUTE);
        }
        else
            rrdset_next(st_mem_ksm_ratios);

        rrddim_set_by_pointer(st_mem_ksm_ratios, rd_savings, (saved * 1000000) / offered);

        rrdset_done(st_mem_ksm_ratios);
    }

    return 0;
}
