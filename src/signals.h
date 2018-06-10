// SPDX-License-Identifier: GPL-3.0+
#ifndef NETDATA_SIGNALS_H
#define NETDATA_SIGNALS_H

extern void signals_init(void);
extern void signals_block(void);
extern void signals_unblock(void);
extern void signals_reset(void);
extern void signals_handle(void) NORETURN;

#endif //NETDATA_SIGNALS_H
