// SPDX-License-Identifier: GPL-3.0-or-later

// Linux audit subsystem status collector.
// Queries the kernel audit status via NETLINK_AUDIT socket (AUDIT_GET)
// and exposes backlog depth, lost events, configuration, and failure mode.

#include "debugfs_plugin.h"

#include <linux/audit.h>
#include <linux/netlink.h>
#include <sys/socket.h>
#include <string.h>
#include <unistd.h>

// -----------------------------------------------------------------------
// netlink audit query

struct audit_reply {
    int                  valid;       // whether the query succeeded
    uint32_t             enabled;     // 0=disabled, 1=enabled, 2=immutable
    uint32_t             failure;     // 0=silent, 1=printk, 2=panic
    uint32_t             pid;         // audit daemon pid (0=no daemon)
    uint32_t             rate_limit;  // max events/s (0=unlimited)
    uint32_t             backlog_limit;
    uint32_t             lost;        // cumulative lost events
    uint32_t             backlog;     // current queue depth
};

// query the kernel audit status via netlink
// returns 0 on success, -1 on failure
static int audit_netlink_query(struct audit_reply *reply) {
    memset(reply, 0, sizeof(*reply));

    int fd = socket(PF_NETLINK, SOCK_RAW | SOCK_CLOEXEC, NETLINK_AUDIT);
    if (fd < 0)
        return -1;

    // bind to the netlink socket
    struct sockaddr_nl addr = {
        .nl_family = AF_NETLINK,
        .nl_pid    = (uint32_t)getpid(),
        .nl_groups = 0,
    };
    if (bind(fd, (struct sockaddr *)&addr, sizeof(addr)) < 0) {
        close(fd);
        return -1;
    }

    // send AUDIT_GET request
    struct {
        struct nlmsghdr nlh;
        struct audit_status s;
    } req = {
        .nlh = {
            .nlmsg_len   = NLMSG_LENGTH(sizeof(struct audit_status)),
            .nlmsg_type  = AUDIT_GET,
            .nlmsg_flags = NLM_F_REQUEST | NLM_F_ACK,
            .nlmsg_seq   = 1,
            .nlmsg_pid   = (uint32_t)getpid(),
        },
        .s = { 0 },
    };

    if (send(fd, &req, req.nlh.nlmsg_len, 0) < 0) {
        close(fd);
        return -1;
    }

    // receive response
    char buf[8192];
    struct sockaddr_nl from;
    socklen_t fromlen = sizeof(from);

    // set a 2-second timeout to avoid blocking forever
    struct timeval tv = { .tv_sec = 2, .tv_usec = 0 };
    setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv));

    for (int attempts = 0; attempts < 5; attempts++) {
        ssize_t len = recvfrom(fd, buf, sizeof(buf), 0, (struct sockaddr *)&from, &fromlen);
        if (len < 0) {
            close(fd);
            return -1;
        }

        struct nlmsghdr *nlh = (struct nlmsghdr *)buf;
        if (!NLMSG_OK(nlh, (size_t)len))
            continue;

        if (nlh->nlmsg_type == AUDIT_GET) {
            struct audit_status *s = NLMSG_DATA(nlh);
            reply->valid         = 1;
            reply->enabled       = s->enabled;
            reply->failure       = s->failure;
            reply->pid           = s->pid;
            reply->rate_limit    = s->rate_limit;
            reply->backlog_limit = s->backlog_limit;
            reply->lost          = s->lost;
            reply->backlog       = s->backlog;
            close(fd);
            return 0;
        }

        // skip NLMSG_ERROR (ACK) and other message types
        if (nlh->nlmsg_type == NLMSG_ERROR)
            continue;
    }

    close(fd);
    return -1;
}

// -----------------------------------------------------------------------
// charts

static int charts_created = 0;

static void audit_send_charts(int update_every, const char *name) {
    if (charts_created)
        return;

    charts_created = 1;

    netdata_mutex_lock(&stdout_mutex);

    // chart: audit backlog depth
    printf(PLUGINSD_KEYWORD_CHART
        " audit.backlog '' 'Audit Backlog' 'events' 'audit' 'audit.backlog' line 1340 %d '' 'debugfs.plugin' '%s'\n",
        update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'backlog' 'backlog' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    // chart: audit backlog utilization (percentage)
    printf(PLUGINSD_KEYWORD_CHART
        " audit.backlog_utilization '' 'Audit Backlog Utilization' '%%' 'audit' 'audit.backlog_utilization' area 1341 %d '' 'debugfs.plugin' '%s'\n",
        update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'utilization' 'utilization' %s 1 100 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    // chart: audit backlog limit
    printf(PLUGINSD_KEYWORD_CHART
        " audit.backlog_limit '' 'Audit Backlog Limit' 'events' 'audit' 'audit.backlog_limit' line 1342 %d '' 'debugfs.plugin' '%s'\n",
        update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'backlog_limit' 'backlog_limit' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    // chart: audit lost events
    printf(PLUGINSD_KEYWORD_CHART
        " audit.lost '' 'Audit Lost Events' 'events/s' 'audit' 'audit.lost' area 1343 %d '' 'debugfs.plugin' '%s'\n",
        update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'lost' 'lost' %s 1 1 ''\n",
        RRD_ALGORITHM_INCREMENTAL_NAME);

    // chart: audit status
    printf(PLUGINSD_KEYWORD_CHART
        " audit.status '' 'Audit Status' 'state' 'audit' 'audit.status' line 1344 %d '' 'debugfs.plugin' '%s'\n",
        update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'enabled' 'enabled' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'failure' 'failure' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'rate_limit' 'rate_limit' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'pid' 'pid' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

static void audit_send_data(struct audit_reply *r) {
    netdata_mutex_lock(&stdout_mutex);

    // backlog depth
    printf(PLUGINSD_KEYWORD_BEGIN " audit.backlog\n");
    printf(PLUGINSD_KEYWORD_SET " backlog = %u\n", r->backlog);
    printf(PLUGINSD_KEYWORD_END "\n");

    // backlog utilization
    collected_number utilization = 0;
    if (r->backlog_limit > 0)
        utilization = (collected_number)r->backlog * 10000 / (collected_number)r->backlog_limit;
    printf(PLUGINSD_KEYWORD_BEGIN " audit.backlog_utilization\n");
    printf(PLUGINSD_KEYWORD_SET " utilization = %lld\n", utilization);
    printf(PLUGINSD_KEYWORD_END "\n");

    // backlog limit
    printf(PLUGINSD_KEYWORD_BEGIN " audit.backlog_limit\n");
    printf(PLUGINSD_KEYWORD_SET " backlog_limit = %u\n", r->backlog_limit);
    printf(PLUGINSD_KEYWORD_END "\n");

    // lost events (incremental)
    printf(PLUGINSD_KEYWORD_BEGIN " audit.lost\n");
    printf(PLUGINSD_KEYWORD_SET " lost = %u\n", r->lost);
    printf(PLUGINSD_KEYWORD_END "\n");

    // status
    printf(PLUGINSD_KEYWORD_BEGIN " audit.status\n");
    printf(PLUGINSD_KEYWORD_SET " enabled = %u\n", r->enabled);
    printf(PLUGINSD_KEYWORD_SET " failure = %u\n", r->failure);
    printf(PLUGINSD_KEYWORD_SET " rate_limit = %u\n", r->rate_limit);
    printf(PLUGINSD_KEYWORD_SET " pid = %u\n", r->pid);
    printf(PLUGINSD_KEYWORD_END "\n");

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

// -----------------------------------------------------------------------
// module entry point

int do_module_audit(int update_every, const char *name) {
    static int check_if_available = 1;

    if (check_if_available) {
        struct audit_reply r;
        if (audit_netlink_query(&r) < 0 || !r.valid) {
            netdata_log_info("audit: netlink AUDIT_GET query failed, audit module disabled");
            return 1; // disable this module
        }
        check_if_available = 0;
    }

    struct audit_reply reply;
    if (audit_netlink_query(&reply) < 0 || !reply.valid)
        return 0; // transient failure, keep module enabled

    audit_send_charts(update_every, name);
    audit_send_data(&reply);

    return 0;
}
