// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

static int fd_is_valid(int fd) {
    errno_clear();
    return fcntl(fd, F_GETFD) != -1 || errno != EBADF;
}

static void setcloexec(int fd) {
    int flags = fcntl(fd, F_GETFD);
    if (flags != -1)
        (void) fcntl(fd, F_SETFD, flags | FD_CLOEXEC);
}

int os_get_fd_open_max(void) {
    static int fd_open_max = CLOSE_RANGE_FD_MAX;

    if(fd_open_max != CLOSE_RANGE_FD_MAX)
        return fd_open_max;

    if(fd_open_max == CLOSE_RANGE_FD_MAX || fd_open_max == -1) {
        struct rlimit rl;
        if (getrlimit(RLIMIT_NOFILE, &rl) == 0 && rl.rlim_max != RLIM_INFINITY)
            fd_open_max = rl.rlim_max;
    }

#ifdef _SC_OPEN_MAX
    if(fd_open_max == CLOSE_RANGE_FD_MAX || fd_open_max == -1) {
        fd_open_max = sysconf(_SC_OPEN_MAX);
    }
#endif

    if(fd_open_max == CLOSE_RANGE_FD_MAX || fd_open_max == -1) {
        // Arbitrary default if everything else fails
        fd_open_max = 65535;
    }

    return fd_open_max;
}

void os_close_range(int first, int last, int flags) {
#if defined(HAVE_CLOSE_RANGE)
    if(close_range(first, last, flags) == 0) return;
#endif

#if defined(OS_LINUX)
    DIR *dir = opendir("/proc/self/fd");
    if (dir != NULL) {
        struct dirent *entry;
        while ((entry = readdir(dir)) != NULL) {
            int fd = str2i(entry->d_name);
            if (fd >= first && (last == CLOSE_RANGE_FD_MAX || fd <= last) && fd_is_valid(fd)) {
                if(flags & CLOSE_RANGE_CLOEXEC)
                    setcloexec(fd);
                else
                    (void)close(fd);
            }
        }
        closedir(dir);
        return;
    }
#endif

    // Fallback to looping through all file descriptors if necessary
    if (last == CLOSE_RANGE_FD_MAX)
        last = os_get_fd_open_max();

    for (int fd = first; fd <= last; fd++) {
        if (fd_is_valid(fd)) {
            if(flags & CLOSE_RANGE_CLOEXEC)
                setcloexec(fd);
            else
                (void)close(fd);
        }
    }
}

static int compare_ints(const void *a, const void *b) {
    int int_a = *((int*)a);
    int int_b = *((int*)b);
    return (int_a > int_b) - (int_a < int_b);
}

void os_close_all_non_std_open_fds_except(const int fds[], size_t fds_num, int flags) {
    if (fds_num == 0 || fds == NULL) {
        os_close_range(STDERR_FILENO + 1, CLOSE_RANGE_FD_MAX, flags);
        return;
    }

    // copy the fds array to ensure we will not alter them
    int fds_copy[fds_num];
    memcpy(fds_copy, fds, sizeof(fds_copy));

    qsort(fds_copy, fds_num, sizeof(int), compare_ints);

    int start = STDERR_FILENO + 1;
    size_t i = 0;

    // filter out all fds with a number smaller than our start
    for (; i < fds_num; i++)
        if(fds_copy[i] >= start) break;

    // call os_close_range() as many times as needed
    for (; i < fds_num; i++) {
        if (fds_copy[i] > start)
            os_close_range(start, fds_copy[i] - 1, flags);

        start = fds_copy[i] + 1;
    }

    os_close_range(start, CLOSE_RANGE_FD_MAX, flags);
}
