// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNALS_H
#define NETDATA_SIGNALS_H 1

void nd_initialize_signals(void);
void nd_process_signals(void) NORETURN;

#endif //NETDATA_SIGNALS_H
