// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef CLOSE_RANGE_H
#define CLOSE_RANGE_H

#define CLOSE_RANGE_FD_MAX (int)(~0U)

int os_get_fd_open_max(void);
void os_close_range(int first, int last);
void os_close_all_non_std_open_fds_except(const int fds[], size_t fds_num);

#endif //CLOSE_RANGE_H
