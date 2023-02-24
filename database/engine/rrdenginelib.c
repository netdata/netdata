// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

int check_file_properties(uv_file file, uint64_t *file_size, size_t min_size)
{
    int ret;
    uv_fs_t req;
    uv_stat_t* s;

    ret = uv_fs_fstat(NULL, &req, file, NULL);
    if (ret < 0) {
        fatal("uv_fs_fstat: %s\n", uv_strerror(ret));
    }
    fatal_assert(req.result == 0);
    s = req.ptr;
    if (!(s->st_mode & S_IFREG)) {
        error("Not a regular file.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    if (s->st_size < min_size) {
        error("File length is too short.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    *file_size = s->st_size;
    uv_fs_req_cleanup(&req);

    return 0;
}

/**
 * Open file for I/O.
 *
 * @param path The full path of the file.
 * @param flags Same flags as the open() system call uses.
 * @param file On success sets (*file) to be the uv_file that was opened.
 * @param direct Tries to open a file in direct I/O mode when direct=1, falls back to buffered mode if not possible.
 * @return Returns UV error number that is < 0 on failure. 0 on success.
 */
int open_file_for_io(char *path, int flags, uv_file *file, int direct)
{
    uv_fs_t req;
    int fd = -1, current_flags;

    fatal_assert(0 == direct || 1 == direct);
    for ( ; direct >= 0 ; --direct) {
#ifdef __APPLE__
        /* Apple OS does not support O_DIRECT */
        direct = 0;
#endif
        current_flags = flags;
        if (direct) {
            current_flags |= O_DIRECT;
        }
        fd = uv_fs_open(NULL, &req, path, current_flags, S_IRUSR | S_IWUSR, NULL);
        if (fd < 0) {
            if ((direct) && (UV_EINVAL == fd)) {
                error("File \"%s\" does not support direct I/O, falling back to buffered I/O.", path);
            } else {
                error("Failed to open file \"%s\".", path);
                --direct; /* break the loop */
            }
        } else {
            fatal_assert(req.result >= 0);
            *file = req.result;
#ifdef __APPLE__
            info("Disabling OS X caching for file \"%s\".", path);
            fcntl(fd, F_NOCACHE, 1);
#endif
            --direct; /* break the loop */
        }
        uv_fs_req_cleanup(&req);
    }

    return fd;
}

int is_legacy_child(const char *machine_guid)
{
    uuid_t uuid;
    char  dbengine_file[FILENAME_MAX+1];

    if (unlikely(!strcmp(machine_guid, "unittest-dbengine") || !strcmp(machine_guid, "dbengine-dataset") ||
                 !strcmp(machine_guid, "dbengine-stress-test"))) {
        return 1;
    }
    if (!uuid_parse(machine_guid, uuid)) {
        uv_fs_t stat_req;
        snprintfz(dbengine_file, FILENAME_MAX, "%s/%s/dbengine", netdata_configured_cache_dir, machine_guid);
        int rc = uv_fs_stat(NULL, &stat_req, dbengine_file, NULL);
        if (likely(rc == 0 && ((stat_req.statbuf.st_mode & S_IFMT) == S_IFDIR))) {
            //info("Found legacy engine folder \"%s\"", dbengine_file);
            return 1;
        }
    }
    return 0;
}

int count_legacy_children(char *dbfiles_path)
{
    int ret;
    uv_fs_t req;
    uv_dirent_t dent;
    int legacy_engines = 0;

    ret = uv_fs_scandir(NULL, &req, dbfiles_path, 0, NULL);
    if (ret < 0) {
        uv_fs_req_cleanup(&req);
        error("uv_fs_scandir(%s): %s", dbfiles_path, uv_strerror(ret));
        return ret;
    }

    while(UV_EOF != uv_fs_scandir_next(&req, &dent)) {
        if (dent.type == UV_DIRENT_DIR) {
            if (is_legacy_child(dent.name))
                legacy_engines++;
        }
    }
    uv_fs_req_cleanup(&req);
    return legacy_engines;
}

int compute_multidb_diskspace()
{
    char multidb_disk_space_file[FILENAME_MAX + 1];
    FILE *fp;
    int computed_multidb_disk_quota_mb = -1;

    snprintfz(multidb_disk_space_file, FILENAME_MAX, "%s/dbengine_multihost_size", netdata_configured_varlib_dir);
    fp = fopen(multidb_disk_space_file, "r");
    if (likely(fp)) {
        int rc = fscanf(fp, "%d", &computed_multidb_disk_quota_mb);
        fclose(fp);
        if (unlikely(rc != 1 || computed_multidb_disk_quota_mb < RRDENG_MIN_DISK_SPACE_MB)) {
            errno = 0;
            error("File '%s' contains invalid input, it will be rebuild", multidb_disk_space_file);
            computed_multidb_disk_quota_mb = -1;
        }
    }

    if (computed_multidb_disk_quota_mb == -1) {
        int rc = count_legacy_children(netdata_configured_cache_dir);
        if (likely(rc >= 0)) {
            computed_multidb_disk_quota_mb = (rc + 1) * default_rrdeng_disk_quota_mb;
            info("Found %d legacy dbengines, setting multidb diskspace to %dMB", rc, computed_multidb_disk_quota_mb);

            fp = fopen(multidb_disk_space_file, "w");
            if (likely(fp)) {
                fprintf(fp, "%d", computed_multidb_disk_quota_mb);
                info("Created file '%s' to store the computed value", multidb_disk_space_file);
                fclose(fp);
            } else
                error("Failed to store the default multidb disk quota size on '%s'", multidb_disk_space_file);
        }
        else
            computed_multidb_disk_quota_mb = default_rrdeng_disk_quota_mb;
    }

    return computed_multidb_disk_quota_mb;
}
