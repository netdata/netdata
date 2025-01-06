// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_SYSTEMD_NOTIFY_H
#define ND_SYSTEMD_NOTIFY_H 1

int notify_ready(void);
int notify_reloading(void);
int notify_stopping(void);

#endif /* ND_SYSTEMD_NOTIFY_H */
