// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_freebsd.h"

#include <sys/mount.h>

struct mount_point {
    char *name;
    uint32_t hash;
    size_t len;

    // flags
    int configured;
    int enabled;
    int updated;

    int do_space;
    int do_inodes;

    size_t collected; // the number of times this has been collected

    // charts and dimensions

    RRDSET *st_space;
    RRDDIM *rd_space_used;
    RRDDIM *rd_space_avail;
    RRDDIM *rd_space_reserved;

    RRDSET *st_inodes;
    RRDDIM *rd_inodes_used;
    RRDDIM *rd_inodes_avail;

    struct mount_point *next;
};

static struct mount_point *mount_points_root = NULL, *mount_points_last_used = NULL;

static size_t mount_points_added = 0, mount_points_found = 0;

static void mount_point_free(struct mount_point *m) {
    if (likely(m->st_space))
        rrdset_is_obsolete(m->st_space);
    if (likely(m->st_inodes))
        rrdset_is_obsolete(m->st_inodes);

    mount_points_added--;
    freez(m->name);
    freez(m);
}

static void mount_points_cleanup() {
    if (likely(mount_points_found == mount_points_added)) return;

    struct mount_point *m = mount_points_root, *last = NULL;
    while(m) {
        if (unlikely(!m->updated)) {
            // info("Removing mount point '%s', linked after '%s'", m->name, last?last->name:"ROOT");

            if (mount_points_last_used == m)
                mount_points_last_used = last;

            struct mount_point *t = m;

            if (m == mount_points_root || !last)
                mount_points_root = m = m->next;

            else
                last->next = m = m->next;

            t->next = NULL;
            mount_point_free(t);
        }
        else {
            last = m;
            m->updated = 0;
            m = m->next;
        }
    }
}

static struct mount_point *get_mount_point(const char *name) {
    struct mount_point *m;

    uint32_t hash = simple_hash(name);

    // search it, from the last position to the end
    for(m = mount_points_last_used ; m ; m = m->next) {
        if (unlikely(hash == m->hash && !strcmp(name, m->name))) {
            mount_points_last_used = m->next;
            return m;
        }
    }

    // search it from the beginning to the last position we used
    for(m = mount_points_root ; m != mount_points_last_used ; m = m->next) {
        if (unlikely(hash == m->hash && !strcmp(name, m->name))) {
            mount_points_last_used = m->next;
            return m;
        }
    }

    // create a new one
    m = callocz(1, sizeof(struct mount_point));
    m->name = strdupz(name);
    m->hash = simple_hash(m->name);
    m->len = strlen(m->name);
    mount_points_added++;

    // link it to the end
    if (mount_points_root) {
        struct mount_point *e;
        for(e = mount_points_root; e->next ; e = e->next) ;
        e->next = m;
    }
    else
        mount_points_root = m;

    return m;
}

// --------------------------------------------------------------------------------------------------------------------
// getmntinfo

int do_getmntinfo(int update_every, usec_t dt) {
    (void)dt;

#define DELAULT_EXCLUDED_PATHS "/proc/*"
// taken from gnulib/mountlist.c and shortened to FreeBSD related fstypes
#define DEFAULT_EXCLUDED_FILESYSTEMS "autofs procfs subfs devfs none"
#define CONFIG_SECTION_GETMNTINFO "plugin:freebsd:getmntinfo"

    static int enable_new_mount_points = -1;
    static int do_space = -1, do_inodes = -1;
    static SIMPLE_PATTERN *excluded_mountpoints = NULL;
    static SIMPLE_PATTERN *excluded_filesystems = NULL;

    if (unlikely(enable_new_mount_points == -1)) {
        enable_new_mount_points = config_get_boolean_ondemand(CONFIG_SECTION_GETMNTINFO,
                                                              "enable new mount points detected at runtime",
                                                              CONFIG_BOOLEAN_AUTO);

        do_space  = config_get_boolean_ondemand(CONFIG_SECTION_GETMNTINFO, "space usage for all disks",  CONFIG_BOOLEAN_AUTO);
        do_inodes = config_get_boolean_ondemand(CONFIG_SECTION_GETMNTINFO, "inodes usage for all disks", CONFIG_BOOLEAN_AUTO);

        excluded_mountpoints = simple_pattern_create(
                config_get(CONFIG_SECTION_GETMNTINFO, "exclude space metrics on paths",
                           DELAULT_EXCLUDED_PATHS)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );

        excluded_filesystems = simple_pattern_create(
                config_get(CONFIG_SECTION_GETMNTINFO, "exclude space metrics on filesystems",
                           DEFAULT_EXCLUDED_FILESYSTEMS)
                , NULL
                , SIMPLE_PATTERN_EXACT
        );
    }

    if (likely(do_space || do_inodes)) {
        struct statfs *mntbuf;
        int mntsize;

        // there is no mount info in sysctl MIBs
        if (unlikely(!(mntsize = getmntinfo(&mntbuf, MNT_NOWAIT)))) {
            error("FREEBSD: getmntinfo() failed");
            do_space = 0;
            error("DISABLED: disk_space.* charts");
            do_inodes = 0;
            error("DISABLED: disk_inodes.* charts");
            error("DISABLED: getmntinfo module");
            return 1;
        } else {
            int i;

            mount_points_found = 0;

            for (i = 0; i < mntsize; i++) {
                char title[4096 + 1];

                struct mount_point *m = get_mount_point(mntbuf[i].f_mntonname);
                m->updated = 1;
                mount_points_found++;

                if (unlikely(!m->configured)) {
                    char var_name[4096 + 1];

                    // this is the first time we see this filesystem

                    // remember we configured it
                    m->configured = 1;

                    m->enabled = enable_new_mount_points;

                    if (likely(m->enabled))
                        m->enabled = !(simple_pattern_matches(excluded_mountpoints, mntbuf[i].f_mntonname)
                                       || simple_pattern_matches(excluded_filesystems, mntbuf[i].f_fstypename));

                    snprintfz(var_name, 4096, "%s:%s", CONFIG_SECTION_GETMNTINFO, mntbuf[i].f_mntonname);
                    m->enabled = config_get_boolean_ondemand(var_name, "enabled", m->enabled);

                    if (unlikely(m->enabled == CONFIG_BOOLEAN_NO))
                        continue;

                    m->do_space  = config_get_boolean_ondemand(var_name, "space usage",  do_space);
                    m->do_inodes = config_get_boolean_ondemand(var_name, "inodes usage", do_inodes);
                }

                if (unlikely(!m->enabled))
                    continue;

                if (unlikely(mntbuf[i].f_flags & MNT_RDONLY && !m->collected))
                    continue;

                // --------------------------------------------------------------------------

                int rendered = 0;

                if (m->do_space == CONFIG_BOOLEAN_YES || (m->do_space == CONFIG_BOOLEAN_AUTO && (mntbuf[i].f_blocks > 2))) {
                    if (unlikely(!m->st_space)) {
                        snprintfz(title, 4096, "Disk Space Usage for %s [%s]",
                                  mntbuf[i].f_mntonname, mntbuf[i].f_mntfromname);
                        m->st_space = rrdset_create_localhost("disk_space",
                                                              mntbuf[i].f_mntonname,
                                                              NULL,
                                                              mntbuf[i].f_mntonname,
                                                              "disk.space",
                                                              title,
                                                              "GB",
                                "freebsd.plugin",
                                                              "getmntinfo",
                                NETDATA_CHART_PRIO_DISKSPACE_SPACE,
                                                              update_every,
                                                              RRDSET_TYPE_STACKED
                        );

                        m->rd_space_avail    = rrddim_add(m->st_space, "avail", NULL,
                                                          mntbuf[i].f_bsize, GIGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                        m->rd_space_used     = rrddim_add(m->st_space, "used", NULL,
                                                          mntbuf[i].f_bsize, GIGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                        m->rd_space_reserved = rrddim_add(m->st_space, "reserved_for_root", "reserved for root",
                                                          mntbuf[i].f_bsize, GIGA_FACTOR, RRD_ALGORITHM_ABSOLUTE);
                    } else
                        rrdset_next(m->st_space);

                    rrddim_set_by_pointer(m->st_space, m->rd_space_avail,    (collected_number) mntbuf[i].f_bavail);
                    rrddim_set_by_pointer(m->st_space, m->rd_space_used,     (collected_number) (mntbuf[i].f_blocks -
                                                                                                 mntbuf[i].f_bfree));
                    rrddim_set_by_pointer(m->st_space, m->rd_space_reserved, (collected_number) (mntbuf[i].f_bfree -
                                                                                                 mntbuf[i].f_bavail));
                    rrdset_done(m->st_space);

                    rendered++;
                }

                // --------------------------------------------------------------------------

                if (m->do_inodes == CONFIG_BOOLEAN_YES || (m->do_inodes == CONFIG_BOOLEAN_AUTO && (mntbuf[i].f_files > 1))) {
                    if (unlikely(!m->st_inodes)) {
                        snprintfz(title, 4096, "Disk Files (inodes) Usage for %s [%s]",
                                  mntbuf[i].f_mntonname, mntbuf[i].f_mntfromname);
                        m->st_inodes = rrdset_create_localhost("disk_inodes",
                                                               mntbuf[i].f_mntonname,
                                                               NULL,
                                                               mntbuf[i].f_mntonname,
                                                               "disk.inodes",
                                                               title,
                                                               "Inodes",
                                "freebsd.plugin",
                                                               "getmntinfo",
                                NETDATA_CHART_PRIO_DISKSPACE_INODES,
                                                               update_every,
                                                               RRDSET_TYPE_STACKED
                        );

                        m->rd_inodes_avail = rrddim_add(m->st_inodes, "avail", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                        m->rd_inodes_used  = rrddim_add(m->st_inodes, "used",  NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
                    } else
                        rrdset_next(m->st_inodes);

                    rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_avail, (collected_number) mntbuf[i].f_ffree);
                    rrddim_set_by_pointer(m->st_inodes, m->rd_inodes_used,  (collected_number) (mntbuf[i].f_files -
                                                                                                mntbuf[i].f_ffree));
                    rrdset_done(m->st_inodes);

                    rendered++;
                }

                if (likely(rendered))
                    m->collected++;
            }
        }
    } else {
        error("DISABLED: getmntinfo module");
        return 1;
    }

    mount_points_cleanup();

    return 0;
}
