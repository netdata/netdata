#ifndef NETDATA_PROC_SELF_MOUNTINFO_H
#define NETDATA_PROC_SELF_MOUNTINFO_H 1
/**
 * @file proc_self_mountinfo.h
 * @brief This file holds the API, used to find mounted filesystems.
 *
 * mountinfo_read() reads the file /proc/mountinfo and stores it's content in a linked list.
 * Entries of this list can be searched with mountinfo_find*().
 *
 * To free the list, run mountinfo_free().
 */

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

/**
 * @brief Get struct mountinfo for the specified disk.
 *
 * @param root  Head of the list
 * @param major value of st_dev for files on filesystem (see stat(2)).
 * @param minor value of st_dev for files on filesystem (see stat(2)).
 *
 * @return The wanted filesystem or NULL.
 */
extern struct mountinfo *mountinfo_find(struct mountinfo *root, unsigned long major, unsigned long minor);
/**
 * @brief Get a list of mounted filesystems matching the parameters.
 *
 * @param root         Head of the list
 * @param filesystem   filesystem type
 * @param mount_source filesystem-specified information or "none".
 *
 * @return The wanted filesystem list or NULL.
 */
extern struct mountinfo *mountinfo_find_by_filesystem_mount_source(struct mountinfo *root, const char *filesystem, const char *mount_source);
/**
 * @brief Get a list of mounted filesystems matching the parameters.
 *
 * @param root          Head of the list
 * @param filesystem    filesystem type
 * @param super_options per-superblock options
 *
 * @return The wanted filesystem list or NULL.
 */
extern struct mountinfo *mountinfo_find_by_filesystem_super_option(struct mountinfo *root, const char *filesystem, const char *super_options);

/**
 * @brief Free the whole list.
 *
 * @param mi The head of the list.
 */
extern void mountinfo_free(struct mountinfo *mi);

/**
 * @brief Parse /proc/mountinfo and initialize a list containing the parsed information.
 *
 * @see man 3 statvfs
 *
 * @param do_statvfs boolean if statvfs() should be used in addition.
 * @return  The head of the list.
 */
extern struct mountinfo *mountinfo_read(int do_statvfs);

#endif /* NETDATA_PROC_SELF_MOUNTINFO_H */
