#include "plugin_proc.h"

int do_run_reboot_required(int update_every, usec_t dt) {
    (void)dt;

    static char *signal_file_path = NULL;

    if (!signal_file_path) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/var/run/reboot-required");
        signal_file_path = strdupz(filename);
    }

    static RRDSET *st = NULL;
    static RRDDIM *rd_yes = NULL;
    static RRDDIM *rd_no = NULL;

    if (unlikely(!st)) {
        st = rrdset_create_localhost(
            "system",
            "post_update_reboot_status",
            NULL,
            "uptime",
            NULL,
            "Post-Update Reboot Status",
            "status",
            PLUGIN_PROC_NAME,
            "/run/reboot_required",
            NETDATA_CHART_PRIO_SYSTEM_REBOOT_REQUIRED,
            update_every,
            RRDSET_TYPE_LINE);

        rd_yes = rrddim_add(st, "required", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_no = rrddim_add(st, "not_required", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    struct stat buf;
    bool exists = (stat(signal_file_path, &buf) == 0);

    rrddim_set_by_pointer(st, rd_yes, exists ? 1 : 0);
    rrddim_set_by_pointer(st, rd_no, exists ? 0 : 1);
    rrdset_done(st);

    return 0;
}
