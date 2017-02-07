#ifndef NETDATA_PROC_SELF_MOUNTINFO_H
#define NETDATA_PROC_SELF_MOUNTINFO_H 1
/**
 * @file proc_self_mountinfo.h
 * @brief Find all mounted filesystems.
 *
 * mountinfo_read() reads the file /proc/mountinfo and stores it's content in a linked list.
 * Entries of this list can be searched with mountinfo_find*().
 *
 * To free the list, run mountinfo_free().
 */

#define MOUNTINFO_IS_DUMMY      0x00000001 ///< Dummy mountinfo.
#define MOUNTINFO_IS_REMOTE     0x00000002 ///< Mounts a remote device.
#define MOUNTINFO_IS_BIND       0x00000004 ///< Mountinfo is bind.
#define MOUNTINFO_IS_SAME_DEV   0x00000008 ///< Mount is on same dev.
#define MOUNTINFO_NO_STAT       0x00000010 ///< Cannot `stat()` mountinfo.
#define MOUNTINFO_NO_SIZE       0x00000020 ///< Size not available.
#define MOUNTINFO_READONLY      0x00000040 ///< Mounted readonly.

/** One mountpoint */
struct mountinfo {
    long id;                ///< Unique identifier of the mount (may be reused after umount(2)).
    long parentid;          ///< ID of parent mount (or of self for the top of the mount tree).
    unsigned long major;    ///< Value of st_dev for files on filesystem. @see man 2 stat
    unsigned long minor;    ///< Value of st_dev for files on filesystem. @see man 2 stat

    char *persistent_id;         ///< A calculated persistent id for the mount point.
    uint32_t persistent_id_hash; ///< Hash of `president_id`.

    char *root;             ///< of the mount within the filesystem
    uint32_t root_hash;     ///< Hash of `root`.

    char *mount_point;         ///< relative to the process's root
    uint32_t mount_point_hash; ///< Hash of `mount_point`.

    char *mount_options; ///< Per-mount options.

    /// Number of optional fields.
    ///
    /// Optional fields are terminated with a field with contents -.
    /// This counts how many of such fields are there.
    /// Until we found a system where this is non zero we do not support optional fields.
    int optional_fields_count;
/*
    char ***optional_fields; // optional fields: zero or more fields of the form "tag[:value]".
*/
    char *filesystem;          ///< type: name of filesystem in the form "type[.subtype]"
    uint32_t filesystem_hash;  ///< Hash of `filesystem`.

    char *mount_source;         ///< Filesystem-specific information or "none".
    uint32_t mount_source_hash; ///< Hash of `mount_source`.

    char *super_options;    ///< per-superblock options.

    uint32_t flags; ///< MOUNTINFO_*

    dev_t st_dev;           ///< Id of device as given by `stat()`

    struct mountinfo *next; ///< item in list
};

/**
 * Get struct mountinfo for the specified disk.
 *
 * @param root  Head of the list.
 * @param major value of st_dev for files on filesystem @see man 2 stat
 * @param minor value of st_dev for files on filesystem @see man 2 stat
 *
 * @return the wanted filesystem or NULL
 */
extern struct mountinfo *mountinfo_find(struct mountinfo *root, unsigned long major, unsigned long minor);
/**
 * Get a list of mounted filesystems matching the parameters.
 *
 * @param root         Head of the list.
 * @param filesystem   type
 * @param mount_source Filesystem-specified information or "none".
 *
 * @return the wanted filesystem list or NULL
 */
extern struct mountinfo *mountinfo_find_by_filesystem_mount_source(struct mountinfo *root, const char *filesystem, const char *mount_source);
/**
 * Get a list of mounted filesystems matching the parameters.
 *
 * @param root          Head of the list.
 * @param filesystem    type
 * @param super_options Per-superblock options.
 *
 * @return the wanted filesystem list or NULL
 */
extern struct mountinfo *mountinfo_find_by_filesystem_super_option(struct mountinfo *root, const char *filesystem, const char *super_options);

/**
 * Free the whole list.
 *
 * @param mi The head of the list.
 */
extern void mountinfo_free(struct mountinfo *mi);

/**
 * Parse /proc/mountinfo and initialize a list containing the parsed information.
 *
 * @see man 3 statvfs
 *
 * @param do_statvfs boolean if statvfs() should be used in addition.
 * @return  the head of the list
 */
extern struct mountinfo *mountinfo_read(int do_statvfs);

#endif /* NETDATA_PROC_SELF_MOUNTINFO_H */
