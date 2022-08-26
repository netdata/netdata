// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_LIB_DUMMIES_H
#define NETDATA_LIB_DUMMIES_H 1

// callback required by fatal()
void netdata_cleanup_and_exit(int ret)
{
    exit(ret);
}

void send_statistics(const char *action, const char *action_result, const char *action_data)
{
    (void)action;
    (void)action_result;
    (void)action_data;
    return;
}

// callbacks required by popen()
void signals_block(void){};
void signals_unblock(void){};
void signals_reset(void){};

// callback required by eval()
int health_variable_lookup(const char *variable, struct rrdcalc *rc, NETDATA_DOUBLE *result)
{
    (void)variable;
    (void)rc;
    (void)result;
    return 0;
};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

#endif // NETDATA_LIB_DUMMIES_H
