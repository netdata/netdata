// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

static int create_data_file(void)
{
    uv_fs_t req;
    uv_file file;
    int i, ret, fd;
    struct rrdeng_df_sb *superblock;
    uv_buf_t iov;

    fd = uv_fs_open(NULL, &req, DATAFILE, UV_FS_O_DIRECT | UV_FS_O_CREAT | UV_FS_O_RDWR | UV_FS_O_TRUNC,
                    S_IRUSR | S_IWUSR, NULL);
    if (fd < 0) {
        fprintf(stderr, "uv_fs_fsopen: %s\n", uv_strerror(fd));
        exit(fd);
    }
    assert(req.result >= 0);
    file = req.result;
    uv_fs_req_cleanup(&req);

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        return UV_ENOMEM;
    }
    (void) strncpy(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ);
    (void) strncpy(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ);
    superblock->tier = 1;

    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_write(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fprintf(stderr, "uv_fs_write: %s\n", uv_strerror(ret));
        exit(ret);
    }
    if (req.result < 0) {
        fprintf(stderr, "uv_fs_write: %s\n", uv_strerror((int)req.result));
        exit(req.result);
    }
    uv_fs_req_cleanup(&req);
    free(superblock);

    datafile.file = file;
    datafile.pos = sizeof(*superblock);
}

static int check_data_file_superblock(uv_file file)
{
    int ret;
    struct rrdeng_df_sb *superblock;
    uv_buf_t iov;
    uv_fs_t req;

    ret = posix_memalign((void *)&superblock, RRDFILE_ALIGNMENT, sizeof(*superblock));
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        return UV_ENOMEM;
    }
    iov = uv_buf_init((void *)superblock, sizeof(*superblock));

    ret = uv_fs_read(NULL, &req, file, &iov, 1, 0, NULL);
    if (ret < 0) {
        fprintf(stderr, "uv_fs_read: %s\n", uv_strerror(ret));
        uv_fs_req_cleanup(&req);
        goto error;
    }
    assert(req.result >= 0);
    uv_fs_req_cleanup(&req);

    if (strncmp(superblock->magic_number, RRDENG_DF_MAGIC, RRDENG_MAGIC_SZ) ||
        strncmp(superblock->version, RRDENG_DF_VER, RRDENG_VER_SZ) ||
        superblock->tier != 1) {
        fprintf(stderr, "File has invalid superblock.\n");
        ret = UV_EINVAL;
    } else {
        ret = 0;
    }
    error:
    free(superblock);
    return ret;
}

static int load_data_file(void)
{
    uv_fs_t req;
    uv_file file;
    int i, ret, fd;
    uint64_t file_size;

    fd = uv_fs_open(NULL, &req, DATAFILE, UV_FS_O_DIRECT | UV_FS_O_RDWR, S_IRUSR | S_IWUSR, NULL);
    if (fd < 0) {
        if (UV_ENOENT != fd) {
            fprintf(stderr, "uv_fs_fsopen: %s\n", uv_strerror(fd));
            /* File exists but something went wrong */
        }
        uv_fs_req_cleanup(&req);
        return fd;
    }
    assert(req.result >= 0);
    file = req.result;
    uv_fs_req_cleanup(&req);
    fprintf(stderr, "Loading data file \"%s\".\n", DATAFILE);

    ret = check_file_properties(file, &file_size, sizeof(struct rrdeng_df_sb));
    if (ret)
        goto error;
    file_size = ALIGN_BYTES_CEILING(file_size);

    ret = check_data_file_superblock(file);
    if (ret)
        goto error;

    datafile.file = file;
    datafile.pos = file_size;

    fprintf(stderr, "Data file \"%s\" loaded (size:%"PRIu64").\n", DATAFILE, file_size);
    return 0;

    error:
    (void) uv_fs_close(NULL, &req, file, NULL);
    uv_fs_req_cleanup(&req);
    return ret;
}

int init_data_files(void)
{
    int ret;

    ret = load_data_file();
    if (UV_ENOENT == ret) {
        fprintf(stderr, "Data file \"%s\" does not exist, creating.\n", DATAFILE);
        ret = create_data_file();
    }

    return ret;
}