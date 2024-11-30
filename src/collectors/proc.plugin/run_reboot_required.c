#include "plugin_proc.h"

static int is_dir_mounted(const char *path) {
    struct stat stat_buf, parent_stat_buf;
    char parent_path[PATH_MAX];

    if (stat(path, &stat_buf) == -1) {
        return -1;
    }

    snprintfz(parent_path, sizeof(parent_path), "%s/..", path);
    if (stat(parent_path, &parent_stat_buf) == -1) {
        return -1;
    }

    return stat_buf.st_dev != parent_stat_buf.st_dev;
}

int do_run_reboot_required(int update_every, usec_t dt) {
    (void)dt;

    static char *signal_file_path = NULL;
    static RRDSET *st = NULL;
    static RRDDIM *rd_required = NULL;
    static RRDDIM *rd_not_required = NULL;

    if (!signal_file_path) {
        char filename[FILENAME_MAX + 1];
        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/var/run");

        // Disable monitoring if running inside a container and the directory is not mounted from the host
        if (getenv("NETDATA_LISTENER_PORT") != NULL) {
            if (!netdata_configured_host_prefix || !*netdata_configured_host_prefix || is_dir_mounted(filename) != 1) {
                return 1;
            }
        } else if (access("/usr/bin/dpkg", X_OK) != 0) {
            // reboot-required only used by Debian-based systems
            return 1;
        }

        snprintfz(filename, FILENAME_MAX, "%s%s", netdata_configured_host_prefix, "/var/run/reboot-required");
        signal_file_path = strdupz(filename);
    }

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

        rd_not_required = rrddim_add(st, "not_required", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rd_required = rrddim_add(st, "required", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
    }

    struct stat buf;
    bool exists = (stat(signal_file_path, &buf) == 0);

    rrddim_set_by_pointer(st, rd_not_required, exists ? 0 : 1);
    rrddim_set_by_pointer(st, rd_required, exists ? 1 : 0);
    rrdset_done(st);

    return 0;
}
