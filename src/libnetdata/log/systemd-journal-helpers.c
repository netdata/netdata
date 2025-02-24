// SPDX-License-Identifier: GPL-3.0-or-later

#include "systemd-journal-helpers.h"
#include "../libnetdata.h"

bool is_path_unix_socket(const char *path) {
    // Check if the path is valid
    if(!path || !*path)
        return false;

    struct stat statbuf;

    // Use stat to check if the file exists and is a socket
    if (stat(path, &statbuf) == -1)
        // The file does not exist or cannot be accessed
        return false;

    // Check if the file is a socket
    if (S_ISSOCK(statbuf.st_mode))
        return true;

    return false;
}

bool is_stderr_connected_to_journal(void) {
    const char *journal_stream = getenv("JOURNAL_STREAM");
    if (!journal_stream)
        return false; // JOURNAL_STREAM is not set

    struct stat stderr_stat;
    if (fstat(STDERR_FILENO, &stderr_stat) < 0)
        return false; // Error in getting stderr info

    // Parse device and inode from JOURNAL_STREAM
    char *endptr;
    long journal_dev = strtol(journal_stream, &endptr, 10);
    if (*endptr != ':')
        return false; // Format error in JOURNAL_STREAM

    long journal_ino = strtol(endptr + 1, NULL, 10);

    return (stderr_stat.st_dev == (dev_t)journal_dev) && (stderr_stat.st_ino == (ino_t)journal_ino);
}

int journal_direct_fd(const char *path) {
    if(!path || !*path)
        path = JOURNAL_DIRECT_SOCKET;

    if(!is_path_unix_socket(path))
        return -1;

    int fd = socket(AF_UNIX, SOCK_DGRAM| DEFAULT_SOCKET_FLAGS, 0);
    if (fd < 0) return -1;

    sock_setcloexec(fd, true);

    struct sockaddr_un addr;
    memset(&addr, 0, sizeof(struct sockaddr_un));
    addr.sun_family = AF_UNIX;
    strncpy(addr.sun_path, path, sizeof(addr.sun_path) - 1);

    // Connect the socket (optional, but can simplify send operations)
    if (connect(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return -1;
    }

    return fd;
}

static inline bool journal_send_with_memfd(int fd __maybe_unused, const char *msg __maybe_unused, size_t msg_len __maybe_unused) {
#if defined(__NR_memfd_create) && defined(MFD_ALLOW_SEALING) && defined(F_ADD_SEALS) && defined(F_SEAL_SHRINK) && defined(F_SEAL_GROW) && defined(F_SEAL_WRITE)
    // Create a memory file descriptor
    int memfd = (int)syscall(__NR_memfd_create, "journald", MFD_ALLOW_SEALING);
    if (memfd < 0) return false;

    // Write data to the memfd
    if (write(memfd, msg, msg_len) != (ssize_t)msg_len) {
        close(memfd);
        return false;
    }

    // Seal the memfd to make it immutable
    if (fcntl(memfd, F_ADD_SEALS, F_SEAL_SHRINK | F_SEAL_GROW | F_SEAL_WRITE) < 0) {
        close(memfd);
        return false;
    }

    struct iovec iov = {0};
    struct msghdr msghdr = {0};
    struct cmsghdr *cmsghdr;
    char cmsgbuf[CMSG_SPACE(sizeof(int))];

    msghdr.msg_iov = &iov;
    msghdr.msg_iovlen = 1;
    msghdr.msg_control = cmsgbuf;
    msghdr.msg_controllen = sizeof(cmsgbuf);

    cmsghdr = CMSG_FIRSTHDR(&msghdr);
    if(!cmsghdr) {
        close(memfd);
        return false;
    }

    cmsghdr->cmsg_level = SOL_SOCKET;
    cmsghdr->cmsg_type = SCM_RIGHTS;
    cmsghdr->cmsg_len = CMSG_LEN(sizeof(int));
    memcpy(CMSG_DATA(cmsghdr), &memfd, sizeof(int));

    ssize_t r = sendmsg(fd, &msghdr, 0);

    close(memfd);
    return r >= 0;
#else
    return false;
#endif
}

bool journal_direct_send(int fd, const char *msg, size_t msg_len) {
    // Send the datagram
    if (send(fd, msg, msg_len, 0) < 0) {
        if(errno != EMSGSIZE)
            return false;

        // datagram is too large, fallback to memfd
        if(!journal_send_with_memfd(fd, msg, msg_len))
            return false;
    }

    return true;
}

void journal_construct_path(char *dst, size_t dst_len, const char *host_prefix, const char *namespace_str) {
    if(!host_prefix)
        host_prefix = "";

    if(namespace_str)
        snprintfz(dst, dst_len, "%s/run/systemd/journal.%s/socket",
                  host_prefix, namespace_str);
    else
        snprintfz(dst, dst_len, "%s" JOURNAL_DIRECT_SOCKET,
                  host_prefix);
}
