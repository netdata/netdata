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
        netdata_log_error("Not a regular file.\n");
        uv_fs_req_cleanup(&req);
        return UV_EINVAL;
    }
    if (s->st_size < min_size) {
        netdata_log_error("File length is too short.\n");
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
                netdata_log_error("File \"%s\" does not support direct I/O, falling back to buffered I/O.", path);
            } else {
                netdata_log_error("Failed to open file \"%s\".", path);
                --direct; /* break the loop */
            }
        } else {
            fatal_assert(req.result >= 0);
            *file = req.result;
#ifdef __APPLE__
            netdata_log_info("Disabling OS X caching for file \"%s\".", path);
            fcntl(fd, F_NOCACHE, 1);
#endif
            --direct; /* break the loop */
        }
        uv_fs_req_cleanup(&req);
    }

    return fd;
}

