// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNAL_HANDLER_H
#define NETDATA_SIGNAL_HANDLER_H 1

void nd_initialize_signals(void);
void nd_process_signals(void) NORETURN;

#endif //NETDATA_SIGNAL_HANDLER_H
