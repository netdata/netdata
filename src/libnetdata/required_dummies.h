// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_DUMMIES_H
#define NETDATA_LIB_DUMMIES_H 1

void rrdset_thread_rda_free(void){}
void sender_thread_buffer_free(void){}
void query_target_free(void){}
void service_exits(void){}
void rrd_collector_finished(void){}

// required by get_system_cpus()
const char *netdata_configured_host_prefix = "";

#endif // NETDATA_LIB_DUMMIES_H
