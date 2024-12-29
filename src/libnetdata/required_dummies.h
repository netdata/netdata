// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_DUMMIES_H
#define NETDATA_LIB_DUMMIES_H 1

// callback required by fatal()
void netdata_cleanup_and_exit(int ret, const char *action, const char *action_result, const char *action_data)
{
    (void)action;
    (void)action_result;
    (void)action_data;

    exit(ret);
}

void rrdset_thread_rda_free(void){}
void sender_thread_buffer_free(void){}
void query_target_free(void){}
void service_exits(void){}
void rrd_collector_finished(void){}

// required by get_system_cpus()
const char *netdata_configured_host_prefix = "";

#endif // NETDATA_LIB_DUMMIES_H
