// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_SIGNAL_HANDLER_H
#define NETDATA_SIGNAL_HANDLER_H 1

void nd_cleanup_deadly_signals(void);
void nd_initialize_signals(bool chain_existing);
void nd_process_signals(void) NORETURN;
#if defined(OS_WINDOWS)
void nd_windows_signal_shutdown_complete(void);
#endif

#endif //NETDATA_SIGNAL_HANDLER_H
