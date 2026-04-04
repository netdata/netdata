// SPDX-License-Identifier: GPL-3.0-or-later

// Linux audit subsystem status collector.
// Queries the kernel audit status via NETLINK_AUDIT socket (AUDIT_GET)
// and exposes backlog depth, lost events, configuration, and failure mode.

#include "debugfs_plugin.h"

#include <linux/audit.h>
#include <linux/netlink.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>

#define AUDIT_STATUS_MIN_PAYLOAD 32  // 8 fields (mask through backlog) = 32 bytes
#define AUDIT_RECV_TIMEOUT_MS    500 // netlink receive timeout in milliseconds
#define AUDIT_RECV_MAX_ATTEMPTS  5   // max recvfrom attempts per query
#define AUDIT_STARTUP_RETRIES    3   // startup failures before permanent disable

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

    // bind to the netlink socket (nl_pid=0 lets kernel auto-assign a unique port ID)
    struct sockaddr_nl addr = {
        .nl_family = AF_NETLINK,
        .nl_pid    = 0,
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
            .nlmsg_pid   = 0,
        },
        .s = { 0 },
    };

    // send to kernel (nl_pid=0)
    struct sockaddr_nl kernel_addr = {
        .nl_family = AF_NETLINK,
        .nl_pid    = 0,
        .nl_groups = 0,
    };
    if (sendto(fd, &req, req.nlh.nlmsg_len, 0,
               (struct sockaddr *)&kernel_addr, sizeof(kernel_addr)) < 0) {
        close(fd);
        return -1;
    }

    // receive response
    char buf[8192];

    // set a timeout to avoid blocking the plugin's collection loop
    struct timeval tv = { .tv_sec = 0, .tv_usec = AUDIT_RECV_TIMEOUT_MS * 1000 };
    if (setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &tv, sizeof(tv)) < 0) {
        close(fd);
        return -1;
    }

    for (int attempts = 0; attempts < AUDIT_RECV_MAX_ATTEMPTS; attempts++) {
        struct sockaddr_nl from;
        socklen_t fromlen = sizeof(from);

        ssize_t len = recvfrom(fd, buf, sizeof(buf), 0, (struct sockaddr *)&from, &fromlen);
        if (len < 0) {
            if (errno == EINTR)
                continue;
            close(fd);
            return -1;
        }

        // only accept messages from the kernel
        if (from.nl_pid != 0)
            continue;

        // iterate all messages in the received buffer
        int msg_len = (int)len;
        for (struct nlmsghdr *nlh = (struct nlmsghdr *)buf;
             NLMSG_OK(nlh, msg_len);
             nlh = NLMSG_NEXT(nlh, msg_len)) {

            if (nlh->nlmsg_type == AUDIT_GET) {
                if (nlh->nlmsg_len < NLMSG_LENGTH(AUDIT_STATUS_MIN_PAYLOAD))
                    continue;

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

            if (nlh->nlmsg_type == NLMSG_ERROR) {
                if (nlh->nlmsg_len < NLMSG_LENGTH(sizeof(struct nlmsgerr)))
                    continue;
                struct nlmsgerr *err = NLMSG_DATA(nlh);
                if (err->error == 0)
                    continue; // ACK, keep looking for AUDIT_GET
                close(fd);
                return -1;
            }
        }
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

    // chart: audit backlog (stacked: used + free = backlog_limit)
    printf(PLUGINSD_KEYWORD_CHART
        " audit.backlog '' 'Audit Backlog' 'events' 'audit' 'audit.backlog' %s %d %d '' 'debugfs.plugin' '%s'\n",
        debugfs_rrdset_type_name(RRDSET_TYPE_STACKED), NETDATA_CHART_PRIO_AUDIT_BACKLOG, update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'used' 'used' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'free' 'free' %s 1 1 'hidden'\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    // chart: audit backlog utilization (percentage)
    printf(PLUGINSD_KEYWORD_CHART
        " audit.backlog_utilization '' 'Audit Backlog Utilization' '%%' 'audit' 'audit.backlog_utilization' %s %d %d '' 'debugfs.plugin' '%s'\n",
        debugfs_rrdset_type_name(RRDSET_TYPE_AREA), NETDATA_CHART_PRIO_AUDIT_BACKLOG_UTIL, update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'utilization' 'utilization' %s 1 100 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    // chart: audit lost events
    printf(PLUGINSD_KEYWORD_CHART
        " audit.lost '' 'Audit Lost Events' 'events/s' 'audit' 'audit.lost' %s %d %d '' 'debugfs.plugin' '%s'\n",
        debugfs_rrdset_type_name(RRDSET_TYPE_AREA), NETDATA_CHART_PRIO_AUDIT_LOST, update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'lost' 'lost' %s 1 1 ''\n",
        RRD_ALGORITHM_INCREMENTAL_NAME);

    // chart: audit enabled state (exactly one dimension is 1 at any time)
    printf(PLUGINSD_KEYWORD_CHART
        " audit.enabled '' 'Audit Enabled State' 'state' 'audit' 'audit.enabled' %s %d %d '' 'debugfs.plugin' '%s'\n",
        debugfs_rrdset_type_name(RRDSET_TYPE_LINE), NETDATA_CHART_PRIO_AUDIT_ENABLED, update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'disabled' 'disabled' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'enabled' 'enabled' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'immutable' 'immutable' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    // chart: audit failure mode (exactly one dimension is 1 at any time)
    printf(PLUGINSD_KEYWORD_CHART
        " audit.failure '' 'Audit Failure Mode' 'state' 'audit' 'audit.failure' %s %d %d '' 'debugfs.plugin' '%s'\n",
        debugfs_rrdset_type_name(RRDSET_TYPE_LINE), NETDATA_CHART_PRIO_AUDIT_FAILURE, update_every, name);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'silent' 'silent' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'printk' 'printk' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);
    printf(PLUGINSD_KEYWORD_DIMENSION " 'panic' 'panic' %s 1 1 ''\n",
        RRD_ALGORITHM_ABSOLUTE_NAME);

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

static void audit_send_data(struct audit_reply *r) {
    netdata_mutex_lock(&stdout_mutex);

    // backlog (stacked: used + free = backlog_limit)
    uint32_t free_backlog = (r->backlog_limit > r->backlog) ? r->backlog_limit - r->backlog : 0;
    printf(PLUGINSD_KEYWORD_BEGIN " audit.backlog\n");
    printf(PLUGINSD_KEYWORD_SET " used = %u\n", r->backlog);
    printf(PLUGINSD_KEYWORD_SET " free = %u\n", free_backlog);
    printf(PLUGINSD_KEYWORD_END "\n");

    // backlog utilization (percentage)
    collected_number utilization = 0;
    if (r->backlog_limit > 0)
        utilization = (collected_number)r->backlog * 10000 / (collected_number)r->backlog_limit;
    printf(PLUGINSD_KEYWORD_BEGIN " audit.backlog_utilization\n");
    printf(PLUGINSD_KEYWORD_SET " utilization = %lld\n", utilization);
    printf(PLUGINSD_KEYWORD_END "\n");

    // lost events (incremental)
    printf(PLUGINSD_KEYWORD_BEGIN " audit.lost\n");
    printf(PLUGINSD_KEYWORD_SET " lost = %u\n", r->lost);
    printf(PLUGINSD_KEYWORD_END "\n");

    // enabled state
    printf(PLUGINSD_KEYWORD_BEGIN " audit.enabled\n");
    printf(PLUGINSD_KEYWORD_SET " disabled = %d\n",  r->enabled == 0 ? 1 : 0);
    printf(PLUGINSD_KEYWORD_SET " enabled = %d\n",   r->enabled == 1 ? 1 : 0);
    printf(PLUGINSD_KEYWORD_SET " immutable = %d\n", r->enabled == 2 ? 1 : 0);
    printf(PLUGINSD_KEYWORD_END "\n");

    // failure mode
    printf(PLUGINSD_KEYWORD_BEGIN " audit.failure\n");
    printf(PLUGINSD_KEYWORD_SET " silent = %d\n", r->failure == 0 ? 1 : 0);
    printf(PLUGINSD_KEYWORD_SET " printk = %d\n", r->failure == 1 ? 1 : 0);
    printf(PLUGINSD_KEYWORD_SET " panic = %d\n",  r->failure == 2 ? 1 : 0);
    printf(PLUGINSD_KEYWORD_END "\n");

    fflush(stdout);
    netdata_mutex_unlock(&stdout_mutex);
}

// -----------------------------------------------------------------------
// module entry point

int do_module_audit(int update_every, const char *name) {
    static int startup_retries = AUDIT_STARTUP_RETRIES;

    struct audit_reply reply;
    if (audit_netlink_query(&reply) < 0 || !reply.valid) {
        if (startup_retries > 0) {
            startup_retries--;
            if (startup_retries == 0) {
                netdata_log_info("audit: netlink AUDIT_GET query failed, audit module disabled");
                return 1; // permanently disable after exhausting retries
            }
            return 0; // retry next cycle
        }
        return 0; // transient failure after startup, keep module enabled
    }

    // mark startup as successful
    startup_retries = 0;

    audit_send_charts(update_every, name);
    audit_send_data(&reply);

    return 0;
}
