// SPDX-License-Identifier: GPL-3.0-or-later

#include "../common.h"

#ifndef NETDATA_LOG_SYSTEMD_JOURNAL_HELPERS_H
#define NETDATA_LOG_SYSTEMD_JOURNAL_HELPERS_H

#define JOURNAL_DIRECT_SOCKET "/run/systemd/journal/socket"

void journal_construct_path(char *dst, size_t dst_len, const char *host_prefix, const char *namespace_str);

int journal_direct_fd(const char *path);
bool journal_direct_send(int fd, const char *msg, size_t msg_len);

bool is_path_unix_socket(const char *path);
bool is_stderr_connected_to_journal(void);

#endif // NETDATA_LOG_SYSTEMD_JOURNAL_HELPERS_H
