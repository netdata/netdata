// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNALS_H
#define NETDATA_SIGNALS_H 1

extern void signals_init(void);
extern void signals_block(void);
extern void signals_unblock(void);
extern void signals_reset(void);
extern void signals_handle(void) NORETURN;

#endif //NETDATA_SIGNALS_H
