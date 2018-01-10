#ifndef NETDATA_PROC_SELF_MOUNTINFO_H
#define NETDATA_PROC_SELF_MOUNTINFO_H 1

#define MOUNTINFO_IS_DUMMY      0x00000001
#define MOUNTINFO_IS_REMOTE     0x00000002
#define MOUNTINFO_IS_BIND       0x00000004
#define MOUNTINFO_IS_SAME_DEV   0x00000008
#define MOUNTINFO_NO_STAT       0x00000010
#define MOUNTINFO_NO_SIZE       0x00000020
#define MOUNTINFO_READONLY      0x00000040

struct mountinfo {
    long id;                // mount ID: unique identifier of the mount (may be reused after umount(2)).
    long parentid;          // parent ID: ID of parent mount (or of self for the top of the mount tree).
    unsigned long major;    // major:minor: value of st_dev for files on filesystem (see stat(2)).
    unsigned long minor;

    char *persistent_id;    // a calculated persistent id for the mount point
    uint32_t persistent_id_hash;

    char *root;             // root: root of the mount within the filesystem.
    uint32_t root_hash;

    char *mount_point;      // mount point: mount point relative to the process's root.
    uint32_t mount_point_hash;

    char *mount_options;    // mount options: per-mount options.

    int optional_fields_count;
/*
    char ***optional_fields; // optional fields: zero or more fields of the form "tag[:value]".
*/
    char *filesystem;       // filesystem type: name of filesystem in the form "type[.subtype]".
    uint32_t filesystem_hash;

    char *mount_source;     // mount source: filesystem-specific information or "none".
    uint32_t mount_source_hash;

    char *super_options;    // super options: per-superblock options.

    uint32_t flags;

    dev_t st_dev;           // id of device as given by stat()

    struct mountinfo *next;
};

extern struct mountinfo *mountinfo_find(struct mountinfo *root, unsigned long major, unsigned long minor);
extern struct mountinfo *mountinfo_find_by_filesystem_mount_source(struct mountinfo *root, const char *filesystem, const char *mount_source);
extern struct mountinfo *mountinfo_find_by_filesystem_super_option(struct mountinfo *root, const char *filesystem, const char *super_options);

extern void mountinfo_free_all(struct mountinfo *mi);
extern struct mountinfo *mountinfo_read(int do_statvfs);

#endif /* NETDATA_PROC_SELF_MOUNTINFO_H */