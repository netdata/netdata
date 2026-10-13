// SPDX-License-Identifier: GPL-3.0-or-later

#include "config.h"

#include <errno.h>
#include <fcntl.h>
#include <inttypes.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/stat.h>
#include <unistd.h>

#include "nd-file-reader.h"

bool nd_file_reader_parse(int argc, char **argv, struct nd_file_request *request) {
    *request = (struct nd_file_request){0};
    if (argc == 2 && strcmp(argv[0], "stat") == 0) {
        request->stat_only = true;
        request->path = argv[1];
        return true;
    }
    if (argc != 4 || strcmp(argv[0], "read") != 0 ||
        (strcmp(argv[1], "regular") != 0 && strcmp(argv[1], "stream") != 0))
        return false;

    // Decimal only: no signs, whitespace, prefixes or trailing bytes.
    if (!*argv[2])
        return false;
    for (const char *p = argv[2]; *p; p++) {
        if (*p < '0' || *p > '9')
            return false;
    }
    errno = 0;
    uintmax_t limit = strtoumax(argv[2], NULL, 10);
    if (errno == ERANGE || limit > UINT64_MAX)
        return false;

    request->regular = strcmp(argv[1], "regular") == 0;
    request->limit = (uint64_t)limit;
    request->path = argv[3];
    return true;
}

static bool write_stdout(const void *data, size_t size) {
    const unsigned char *p = data;
    while (size) {
        ssize_t n = write(STDOUT_FILENO, p, size);
        if (n < 0 && errno == EINTR)
            continue;
        if (n <= 0)
            return false;
        p += n;
        size -= (size_t)n;
    }
    return true;
}

static int result(enum nd_file_result code, int error) {
    if (fprintf(stderr, "NDFILE %u %d\n", (unsigned)code, error) < 0)
        return EXIT_FAILURE;
    return code == ND_FILE_OK ? EXIT_SUCCESS : EXIT_FAILURE;
}

static int read_file(const struct nd_file_request *request) {
    int fd = open(request->path, O_RDONLY | (request->regular ? O_NONBLOCK : 0));
    if (fd < 0)
        return result(ND_FILE_OPEN_ERROR, errno);

    enum nd_file_result code = ND_FILE_OK;
    int error = 0;
    struct stat st;
    if (request->regular) {
        if (fstat(fd, &st) != 0) {
            code = ND_FILE_STAT_ERROR;
            error = errno;
        } else if (!S_ISREG(st.st_mode)) {
            code = ND_FILE_NOT_REGULAR;
        } else if (request->limit && st.st_size > 0 && (uint64_t)st.st_size > request->limit) {
            code = ND_FILE_TOO_LARGE;
        }
    }

    unsigned char data[32768];
    uint64_t received = 0;
    bool connected = true;
    while (code == ND_FILE_OK) {
        size_t wanted = sizeof(data);
        if (request->limit && request->limit - received < wanted)
            wanted = (size_t)(request->limit - received) + 1;
        ssize_t n = read(fd, data, wanted);
        if (n < 0) {
            if (errno == EINTR)
                continue;
            code = ND_FILE_READ_ERROR;
            error = errno;
            break;
        }
        if (n == 0)
            break;
        if (request->limit && (uint64_t)n > request->limit - received) {
            code = ND_FILE_TOO_LARGE;
            break;
        }
        if (request->limit)
            received += (uint64_t)n;
        if (!write_stdout(data, (size_t)n)) {
            connected = false;
            break;
        }
    }
    if (close(fd) != 0 && code == ND_FILE_OK) {
        code = ND_FILE_CLOSE_ERROR;
        error = errno;
    }
    return connected ? result(code, error) : EXIT_FAILURE;
}

int nd_file_reader_run(const struct nd_file_request *request) {
    if (!request->stat_only)
        return read_file(request);

    struct stat st;
    if (stat(request->path, &st) != 0)
        return result(ND_FILE_STAT_ERROR, errno);
#ifdef __APPLE__
    struct timespec mtime = st.st_mtimespec;
#else
    struct timespec mtime = st.st_mtim;
#endif
    char data[64];
    int length = snprintf(data, sizeof(data), "%jd %ld\n", (intmax_t)mtime.tv_sec, mtime.tv_nsec);
    if (length < 0 || (size_t)length >= sizeof(data) || !write_stdout(data, (size_t)length))
        return EXIT_FAILURE;
    return result(ND_FILE_OK, 0);
}
