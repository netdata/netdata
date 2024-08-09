// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef CLOSE_RANGE_H
#define CLOSE_RANGE_H

#define CLOSE_RANGE_FD_MAX (int)(~0U)

#ifndef CLOSE_RANGE_UNSHARE
#define CLOSE_RANGE_UNSHARE	(1U << 1)
#endif

#ifndef CLOSE_RANGE_CLOEXEC
#define CLOSE_RANGE_CLOEXEC (1U << 2)
#endif

int os_get_fd_open_max(void);
void os_close_range(int first, int last, int flags);
void os_close_all_non_std_open_fds_except(const int fds[], size_t fds_num, int flags);

#endif //CLOSE_RANGE_H
