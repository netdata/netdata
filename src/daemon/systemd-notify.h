// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_SYSTEMD_NOTIFY_H
#define ND_SYSTEMD_NOTIFY_H 1

#include <inttypes.h>

int notify_ready(void);
int notify_reloading(void);
int notify_extend_timeout(uint64_t);
int notify_stopping(uint64_t);
int notify_status(const char *);

#endif /* ND_SYSTEMD_NOTIFY_H */
