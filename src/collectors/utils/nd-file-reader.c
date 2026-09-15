// SPDX-License-Identifier: GPL-3.0-or-later

#include "config.h"

#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <unistd.h>

#ifdef __linux__
#include <linux/capability.h>
#include <sys/syscall.h>
#endif

#include "exec-signals.h"
#include "nd-file-reader.h"

static uint32_t load_u32(const unsigned char *p) {
    return ((uint32_t)p[0] << 24) | ((uint32_t)p[1] << 16) | ((uint32_t)p[2] << 8) | p[3];
}

static uint64_t load_u64(const unsigned char *p) {
    return ((uint64_t)load_u32(p) << 32) | load_u32(p + 4);
}

static void store_u32(unsigned char *p, uint32_t value) {
    p[0] = (unsigned char)(value >> 24);
    p[1] = (unsigned char)(value >> 16);
    p[2] = (unsigned char)(value >> 8);
    p[3] = (unsigned char)value;
}

static void store_u64(unsigned char *p, uint64_t value) {
    store_u32(p, (uint32_t)(value >> 32));
    store_u32(p + 4, (uint32_t)value);
}

// A clean EOF between requests is distinct from a truncated request.
static int read_exact(void *data, size_t size) {
    unsigned char *p = data;
    size_t received = 0;
    while (received < size) {
        ssize_t n = read(STDIN_FILENO, p + received, size - received);
        if (n == 0)
            return received == 0 ? 0 : -1;
        if (n < 0) {
            if (errno == EINTR)
                continue;
            return -1;
        }
        received += (size_t)n;
    }
    return 1;
}

static bool write_exact(const void *data, size_t size) {
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

static bool frame(uint32_t kind, uint32_t code, int err, const void *data, uint32_t size) {
    unsigned char header[16];
    store_u32(header, kind);
    store_u32(header + 4, size);
    store_u32(header + 8, code);
    store_u32(header + 12, (uint32_t)err);
    return write_exact(header, sizeof(header)) && write_exact(data, size);
}

static bool result(uint32_t code, int err) {
    return frame(ND_FILE_RESULT, code, err, NULL, 0);
}

// This mode reads before exec, so it cannot rely on exec to discard inherited
// capabilities. Sealing IDs also prevents retaining a saved privileged identity.
static bool seal_reader_identity(void) {
    uid_t uid = geteuid();
    gid_t gid = getegid();
    if (uid == 0) {
        errno = EPERM;
        return false;
    }
#ifdef HAVE_SETRESGID
    if (setresgid(gid, gid, gid) != 0)
        return false;
#else
    if (setregid(gid, gid) != 0)
        return false;
#endif
#ifdef HAVE_SETRESUID
    if (setresuid(uid, uid, uid) != 0)
        return false;
#else
    if (setreuid(uid, uid) != 0)
        return false;
#endif

#ifdef __linux__
    // The kernel interface is available even in builds without optional libcap.
    // Clearing permitted/inheritable also clears ambient capabilities, which
    // must be subsets of both sets.
    struct __user_cap_header_struct header = { .version = _LINUX_CAPABILITY_VERSION_3, .pid = 0 };
    struct __user_cap_data_struct caps[2] = {{0}, {0}};
    if (syscall(SYS_capset, &header, caps) != 0 || syscall(SYS_capget, &header, caps) != 0)
        return false;
    if (caps[0].effective || caps[0].permitted || caps[0].inheritable ||
        caps[1].effective || caps[1].permitted || caps[1].inheritable) {
        errno = EPERM;
        return false;
    }
#endif
    if (getuid() != uid || geteuid() != uid || getgid() != gid || getegid() != gid) {
        errno = EPERM;
        return false;
    }
    return true;
}

static bool read_file(const char *path, bool regular, uint64_t limit) {
    int fd = open(path, O_RDONLY | (regular ? O_NONBLOCK : 0));
    if (fd < 0)
        return result(ND_FILE_OPEN_ERROR, errno);

    uint32_t code = ND_FILE_OK;
    int failure = 0;
    struct stat st;
    if (regular) {
        if (fstat(fd, &st) != 0) {
            code = ND_FILE_STAT_ERROR;
            failure = errno;
        } else if (!S_ISREG(st.st_mode)) {
            code = ND_FILE_NOT_REGULAR;
        } else if (limit && st.st_size > 0 && (uint64_t)st.st_size > limit) {
            code = ND_FILE_TOO_LARGE;
        }
    }

    unsigned char data[ND_FILE_CHUNK_SIZE];
    uint64_t received = 0;
    bool connected = true;
    while (code == ND_FILE_OK) {
        size_t wanted = sizeof(data);
        if (limit && limit - received < wanted)
            wanted = (size_t)(limit - received) + 1;
        ssize_t n = read(fd, data, wanted);
        if (n < 0) {
            if (errno == EINTR)
                continue;
            code = ND_FILE_READ_ERROR;
            failure = errno;
            break;
        }
        if (n == 0)
            break;
        if (limit && (uint64_t)n > limit - received) {
            code = ND_FILE_TOO_LARGE;
            break;
        }
        if (limit)
            received += (uint64_t)n;
        if (!frame(ND_FILE_DATA, ND_FILE_OK, 0, data, (uint32_t)n)) {
            connected = false;
            break;
        }
    }

    if (close(fd) != 0 && code == ND_FILE_OK) {
        code = ND_FILE_CLOSE_ERROR;
        failure = errno;
    }
    return connected && result(code, failure);
}

static bool stat_file(const char *path) {
    struct stat st;
    if (stat(path, &st) != 0)
        return result(ND_FILE_STAT_ERROR, errno);

    unsigned char info[16];
#ifdef __APPLE__
    store_u64(info, (uint64_t)st.st_mtimespec.tv_sec);
    store_u64(info + 8, (uint64_t)st.st_mtimespec.tv_nsec);
#else
    store_u64(info, (uint64_t)st.st_mtim.tv_sec);
    store_u64(info + 8, (uint64_t)st.st_mtim.tv_nsec);
#endif
    return frame(ND_FILE_INFO, ND_FILE_OK, 0, info, sizeof(info));
}

int nd_file_reader_main(void) {
    if (!seal_reader_identity()) {
        perror("nd-run: file reader privilege reduction");
        return 1;
    }
    reset_signal_dispositions();

    unsigned char hello[16] = "NDFILE01";
    store_u32(hello + 8, PATH_MAX);
    store_u32(hello + 12, ND_FILE_CHUNK_SIZE);
    if (!write_exact(hello, sizeof(hello)))
        return 1;

    for (;;) {
        unsigned char header[24];
        int received = read_exact(header, sizeof(header));
        if (received <= 0)
            return received == 0 ? 0 : 1;
        uint32_t operation = load_u32(header);
        uint32_t flags = load_u32(header + 4);
        uint64_t limit = load_u64(header + 8);
        uint32_t length = load_u32(header + 16);
        if (load_u32(header + 20) != 0 || length >= PATH_MAX || limit > INT64_MAX ||
            (flags & ~ND_FILE_REGULAR) ||
            (operation != ND_FILE_READ && operation != ND_FILE_STAT) ||
            (operation == ND_FILE_STAT && (flags || limit)))
            return 1;

        char path[PATH_MAX];
        if (read_exact(path, length) != 1 || memchr(path, '\0', length))
            return 1;
        path[length] = '\0';
        bool connected = operation == ND_FILE_READ ? read_file(path, flags & ND_FILE_REGULAR, limit) : stat_file(path);
        if (!connected)
            return 1;
    }
}
