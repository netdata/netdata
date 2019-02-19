// SPDX-License-Identifier: GPL-3.0-or-later

#include "../../libnetdata/libnetdata.h"

#define PLUGIN_NFACCT_NAME "nfacct.plugin"

#define NETDATA_CHART_PRIO_NETFILTER_NEW              8701
#define NETDATA_CHART_PRIO_NETFILTER_CHANGES          8702
#define NETDATA_CHART_PRIO_NETFILTER_EXPECT           8703
#define NETDATA_CHART_PRIO_NETFILTER_ERRORS           8705
#define NETDATA_CHART_PRIO_NETFILTER_SEARCH           8710

#define NETDATA_CHART_PRIO_NETFILTER_PACKETS          8906
#define NETDATA_CHART_PRIO_NETFILTER_BYTES            8907

#ifdef HAVE_LIBMNL
#include <libmnl/libmnl.h>

static inline size_t mnl_buffer_size() {
    long s = MNL_SOCKET_BUFFER_SIZE;
    if(s <= 0) return 8192;
    return (size_t)s;
}

// callback required by fatal()
void netdata_cleanup_and_exit(int ret) {
    exit(ret);
}

void send_statistics( const char *action, const char *action_result, const char *action_data) {
    (void) action;
    (void) action_result;
    (void) action_data;
    return;
}

// callbacks required by popen()
void signals_block(void) {};
void signals_unblock(void) {};
void signals_reset(void) {};

// callback required by eval()
int health_variable_lookup(const char *variable, uint32_t hash, struct rrdcalc *rc, calculated_number *result) {
    (void)variable;
    (void)hash;
    (void)rc;
    (void)result;
    return 0;
};

// required by get_system_cpus()
char *netdata_configured_host_prefix = "";

// Variables

static int debug = 0;

static int netdata_update_every = 1;

// ----------------------------------------------------------------------------
// DO_NFSTAT - collect netfilter connection tracker statistics via netlink
// example: https://github.com/formorer/pkg-conntrack-tools/blob/master/src/conntrack.c

#ifdef HAVE_LINUX_NETFILTER_NFNETLINK_CONNTRACK_H
#define DO_NFSTAT 1

#define RRD_TYPE_NET_STAT_NETFILTER "netfilter"
#define RRD_TYPE_NET_STAT_CONNTRACK "netlink"

#include <linux/netfilter/nfnetlink_conntrack.h>

static struct {
    int update_every;
    char *buf;
    size_t buf_size;
    struct mnl_socket *mnl;
    struct nlmsghdr *nlh;
    struct nfgenmsg *nfh;
    unsigned int seq;
    uint32_t portid;

    struct nlattr *tb[CTA_STATS_MAX+1];
    const char *attr2name[CTA_STATS_MAX+1];
    kernel_uint_t metrics[CTA_STATS_MAX+1];

    struct nlattr *tb_exp[CTA_STATS_EXP_MAX+1];
    const char *attr2name_exp[CTA_STATS_EXP_MAX+1];
    kernel_uint_t metrics_exp[CTA_STATS_EXP_MAX+1];
} nfstat_root = {
        .update_every = 1,
        .buf = NULL,
        .buf_size = 0,
        .mnl = NULL,
        .nlh = NULL,
        .nfh = NULL,
        .seq = 0,
        .portid = 0,
        .tb = {},
        .attr2name = {
                [CTA_STATS_SEARCHED]	   = "searched",
                [CTA_STATS_FOUND]	       = "found",
                [CTA_STATS_NEW]		       = "new",
                [CTA_STATS_INVALID]	       = "invalid",
                [CTA_STATS_IGNORE]	       = "ignore",
                [CTA_STATS_DELETE]	       = "delete",
                [CTA_STATS_DELETE_LIST]	   = "delete_list",
                [CTA_STATS_INSERT]	       = "insert",
                [CTA_STATS_INSERT_FAILED]  = "insert_failed",
                [CTA_STATS_DROP]	       = "drop",
                [CTA_STATS_EARLY_DROP]	   = "early_drop",
                [CTA_STATS_ERROR]	       = "icmp_error",
                [CTA_STATS_SEARCH_RESTART] = "search_restart",
        },
        .metrics = {},
        .tb_exp = {},
        .attr2name_exp = {
                [CTA_STATS_EXP_NEW]	       = "new",
                [CTA_STATS_EXP_CREATE]	   = "created",
                [CTA_STATS_EXP_DELETE]	   = "deleted",
        },
        .metrics_exp = {}
};


static int nfstat_init(int update_every) {
    nfstat_root.update_every = update_every;

    nfstat_root.buf_size = mnl_buffer_size();
    nfstat_root.buf = mallocz(nfstat_root.buf_size);

    nfstat_root.mnl  = mnl_socket_open(NETLINK_NETFILTER);
    if(!nfstat_root.mnl) {
        error("NFSTAT: mnl_socket_open() failed");
        return 1;
    }

    nfstat_root.seq = (unsigned int)now_realtime_sec() - 1;

    if(mnl_socket_bind(nfstat_root.mnl, 0, MNL_SOCKET_AUTOPID) < 0) {
        error("NFSTAT: mnl_socket_bind() failed");
        return 1;
    }
    nfstat_root.portid = mnl_socket_get_portid(nfstat_root.mnl);

    return 0;
}

static struct nlmsghdr * nfct_mnl_nlmsghdr_put(char *buf, uint16_t subsys, uint16_t type, uint8_t family, uint32_t seq) {
    struct nlmsghdr *nlh;
    struct nfgenmsg *nfh;

    nlh = mnl_nlmsg_put_header(buf);
    nlh->nlmsg_type = (subsys << 8) | type;
    nlh->nlmsg_flags = NLM_F_REQUEST|NLM_F_DUMP;
    nlh->nlmsg_seq = seq;

    nfh = mnl_nlmsg_put_extra_header(nlh, sizeof(struct nfgenmsg));
    nfh->nfgen_family = family;
    nfh->version = NFNETLINK_V0;
    nfh->res_id = 0;

    return nlh;
}

static int nfct_stats_attr_cb(const struct nlattr *attr, void *data) {
    const struct nlattr **tb = data;
    int type = mnl_attr_get_type(attr);

    if (mnl_attr_type_valid(attr, CTA_STATS_MAX) < 0)
        return MNL_CB_OK;

    if (mnl_attr_validate(attr, MNL_TYPE_U32) < 0) {
        error("NFSTAT: mnl_attr_validate() failed");
        return MNL_CB_ERROR;
    }

    tb[type] = attr;
    return MNL_CB_OK;
}

static int nfstat_callback(const struct nlmsghdr *nlh, void *data) {
    (void)data;

    struct nfgenmsg *nfg = mnl_nlmsg_get_payload(nlh);

    mnl_attr_parse(nlh, sizeof(*nfg), nfct_stats_attr_cb, nfstat_root.tb);

    // printf("cpu=%-4u\t", ntohs(nfg->res_id));

    int i;
    // add the metrics of this CPU into the metrics
    for (i = 0; i < CTA_STATS_MAX+1; i++) {
        if (nfstat_root.tb[i]) {
            // printf("%s=%u ", nfstat_root.attr2name[i], ntohl(mnl_attr_get_u32(nfstat_root.tb[i])));
            nfstat_root.metrics[i] += ntohl(mnl_attr_get_u32(nfstat_root.tb[i]));
        }
    }
    // printf("\n");

    return MNL_CB_OK;
}

static int nfstat_collect_conntrack() {
    // zero all metrics - we will sum the metrics of all CPUs later
    int i;
    for (i = 0; i < CTA_STATS_MAX+1; i++)
        nfstat_root.metrics[i] = 0;

    // prepare the request
    nfstat_root.nlh = nfct_mnl_nlmsghdr_put(nfstat_root.buf, NFNL_SUBSYS_CTNETLINK, IPCTNL_MSG_CT_GET_STATS_CPU, AF_UNSPEC, nfstat_root.seq);

    // send the request
    if(mnl_socket_sendto(nfstat_root.mnl, nfstat_root.nlh, nfstat_root.nlh->nlmsg_len) < 0) {
        error("NFSTAT: mnl_socket_sendto() failed");
        return 1;
    }

    // get the reply
    ssize_t ret;
    while ((ret = mnl_socket_recvfrom(nfstat_root.mnl, nfstat_root.buf, nfstat_root.buf_size)) > 0) {
        if(mnl_cb_run(
                nfstat_root.buf
                , (size_t)ret
                , nfstat_root.nlh->nlmsg_seq
                , nfstat_root.portid
                , nfstat_callback
                , NULL
        ) <= MNL_CB_STOP)
            break;
    }

    // verify we run without issues
    if (ret == -1) {
        error("NFSTAT: error communicating with kernel. This plugin can only work when netdata runs as root.");
        return 1;
    }

    return 0;
}

static int nfexp_stats_attr_cb(const struct nlattr *attr, void *data)
{
    const struct nlattr **tb = data;
    int type = mnl_attr_get_type(attr);

    if (mnl_attr_type_valid(attr, CTA_STATS_EXP_MAX) < 0)
        return MNL_CB_OK;

    if (mnl_attr_validate(attr, MNL_TYPE_U32) < 0) {
        error("NFSTAT EXP: mnl_attr_validate() failed");
        return MNL_CB_ERROR;
    }

    tb[type] = attr;
    return MNL_CB_OK;
}

static int nfstat_callback_exp(const struct nlmsghdr *nlh, void *data) {
    (void)data;

    struct nfgenmsg *nfg = mnl_nlmsg_get_payload(nlh);

    mnl_attr_parse(nlh, sizeof(*nfg), nfexp_stats_attr_cb, nfstat_root.tb_exp);

    int i;
    for (i = 0; i < CTA_STATS_EXP_MAX+1; i++) {
        if (nfstat_root.tb_exp[i]) {
            nfstat_root.metrics_exp[i] += ntohl(mnl_attr_get_u32(nfstat_root.tb_exp[i]));
        }
    }

    return MNL_CB_OK;
}

static int nfstat_collect_conntrack_expectations() {
    // zero all metrics - we will sum the metrics of all CPUs later
    int i;
    for (i = 0; i < CTA_STATS_EXP_MAX+1; i++)
        nfstat_root.metrics_exp[i] = 0;

    // prepare the request
    nfstat_root.nlh = nfct_mnl_nlmsghdr_put(nfstat_root.buf, NFNL_SUBSYS_CTNETLINK_EXP, IPCTNL_MSG_EXP_GET_STATS_CPU, AF_UNSPEC, nfstat_root.seq);

    // send the request
    if(mnl_socket_sendto(nfstat_root.mnl, nfstat_root.nlh, nfstat_root.nlh->nlmsg_len) < 0) {
        error("NFSTAT: mnl_socket_sendto() failed");
        return 1;
    }

    // get the reply
    ssize_t ret;
    while ((ret = mnl_socket_recvfrom(nfstat_root.mnl, nfstat_root.buf, nfstat_root.buf_size)) > 0) {
        if(mnl_cb_run(
                nfstat_root.buf
                , (size_t)ret
                , nfstat_root.nlh->nlmsg_seq
                , nfstat_root.portid
                , nfstat_callback_exp
                , NULL
        ) <= MNL_CB_STOP)
            break;
    }

    // verify we run without issues
    if (ret == -1) {
        error("NFSTAT: error communicating with kernel. This plugin can only work when netdata runs as root.");
        return 1;
    }

    return 0;
}

static int nfstat_collect() {
    nfstat_root.seq++;

    if(nfstat_collect_conntrack())
        return 1;

    if(nfstat_collect_conntrack_expectations())
        return 1;

    return 0;
}

static void nfstat_send_metrics() {
    static int new_chart_generated = 0, changes_chart_generated = 0, search_chart_generated = 0, errors_chart_generated = 0, expect_chart_generated = 0;

    if(!new_chart_generated) {
        new_chart_generated = 1;

        printf("CHART %s.%s '' 'Connection Tracker New Connections' 'connections/s' %s '' line %d %d %s\n"
               , RRD_TYPE_NET_STAT_NETFILTER
               , RRD_TYPE_NET_STAT_CONNTRACK "_new"
               , RRD_TYPE_NET_STAT_CONNTRACK
               , NETDATA_CHART_PRIO_NETFILTER_NEW
               , nfstat_root.update_every
               , PLUGIN_NFACCT_NAME
        );
        printf("DIMENSION %s '' incremental 1 1\n", nfstat_root.attr2name[CTA_STATS_NEW]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_IGNORE]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_INVALID]);
    }

    printf(
           "BEGIN %s.%s\n"
           , RRD_TYPE_NET_STAT_NETFILTER
           , RRD_TYPE_NET_STAT_CONNTRACK "_new"
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_NEW]
           , (collected_number) nfstat_root.metrics[CTA_STATS_NEW]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_IGNORE]
           , (collected_number) nfstat_root.metrics[CTA_STATS_IGNORE]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_INVALID]
           , (collected_number) nfstat_root.metrics[CTA_STATS_INVALID]
    );
    printf("END\n");

    // ----------------------------------------------------------------

    if(!changes_chart_generated) {
        changes_chart_generated = 1;

        printf("CHART %s.%s '' 'Connection Tracker Changes' 'changes/s' %s '' line %d %d detail %s\n"
               , RRD_TYPE_NET_STAT_NETFILTER
               , RRD_TYPE_NET_STAT_CONNTRACK "_changes"
               , RRD_TYPE_NET_STAT_CONNTRACK
               , NETDATA_CHART_PRIO_NETFILTER_CHANGES
               , nfstat_root.update_every
               , PLUGIN_NFACCT_NAME
        );
        printf("DIMENSION %s '' incremental  1 1\n", nfstat_root.attr2name[CTA_STATS_INSERT]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_DELETE]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_DELETE_LIST]);
    }

    printf(
           "BEGIN %s.%s\n"
           , RRD_TYPE_NET_STAT_NETFILTER
           , RRD_TYPE_NET_STAT_CONNTRACK "_changes"
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_INSERT]
           , (collected_number) nfstat_root.metrics[CTA_STATS_INSERT]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_DELETE]
           , (collected_number) nfstat_root.metrics[CTA_STATS_DELETE]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_DELETE_LIST]
           , (collected_number) nfstat_root.metrics[CTA_STATS_DELETE_LIST]
    );
    printf("END\n");

    // ----------------------------------------------------------------

    if(!search_chart_generated) {
        search_chart_generated = 1;

        printf("CHART %s.%s '' 'Connection Tracker Searches' 'searches/s' %s '' line %d %d detail %s\n"
               , RRD_TYPE_NET_STAT_NETFILTER
               , RRD_TYPE_NET_STAT_CONNTRACK "_search"
               , RRD_TYPE_NET_STAT_CONNTRACK
               , NETDATA_CHART_PRIO_NETFILTER_SEARCH
               , nfstat_root.update_every
               , PLUGIN_NFACCT_NAME
        );
        printf("DIMENSION %s '' incremental  1 1\n", nfstat_root.attr2name[CTA_STATS_SEARCHED]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_SEARCH_RESTART]);
        printf("DIMENSION %s '' incremental  1 1\n", nfstat_root.attr2name[CTA_STATS_FOUND]);
    }

    printf(
           "BEGIN %s.%s\n"
           , RRD_TYPE_NET_STAT_NETFILTER
           , RRD_TYPE_NET_STAT_CONNTRACK "_search"
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_SEARCHED]
           , (collected_number) nfstat_root.metrics[CTA_STATS_SEARCHED]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_SEARCH_RESTART]
           , (collected_number) nfstat_root.metrics[CTA_STATS_SEARCH_RESTART]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_FOUND]
           , (collected_number) nfstat_root.metrics[CTA_STATS_FOUND]
    );
    printf("END\n");

    // ----------------------------------------------------------------

    if(!errors_chart_generated) {
        errors_chart_generated = 1;

        printf("CHART %s.%s '' 'Connection Tracker Errors' 'events/s' %s '' line %d %d detail %s\n"
               , RRD_TYPE_NET_STAT_NETFILTER
               , RRD_TYPE_NET_STAT_CONNTRACK "_errors"
               , RRD_TYPE_NET_STAT_CONNTRACK
               , NETDATA_CHART_PRIO_NETFILTER_ERRORS
               , nfstat_root.update_every
               , PLUGIN_NFACCT_NAME
        );
        printf("DIMENSION %s '' incremental  1 1\n", nfstat_root.attr2name[CTA_STATS_ERROR]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_INSERT_FAILED]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_DROP]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_EARLY_DROP]);
    }

    printf(
           "BEGIN %s.%s\n"
           , RRD_TYPE_NET_STAT_NETFILTER
           , RRD_TYPE_NET_STAT_CONNTRACK "_errors"
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_ERROR]
           , (collected_number) nfstat_root.metrics[CTA_STATS_ERROR]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_INSERT_FAILED]
           , (collected_number) nfstat_root.metrics[CTA_STATS_INSERT_FAILED]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_DROP]
           , (collected_number) nfstat_root.metrics[CTA_STATS_DROP]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_EARLY_DROP]
           , (collected_number) nfstat_root.metrics[CTA_STATS_EARLY_DROP]
    );
    printf("END\n");

    // ----------------------------------------------------------------

    if(!expect_chart_generated) {
        expect_chart_generated = 1;

        printf("CHART %s.%s '' 'Connection Tracker Expectations' 'expectations/s' %s '' line %d %d detail %s\n"
               , RRD_TYPE_NET_STAT_NETFILTER
               , RRD_TYPE_NET_STAT_CONNTRACK "_expect"
               , RRD_TYPE_NET_STAT_CONNTRACK
               , NETDATA_CHART_PRIO_NETFILTER_EXPECT
               , nfstat_root.update_every
               , PLUGIN_NFACCT_NAME
        );
        printf("DIMENSION %s '' incremental  1 1\n", nfstat_root.attr2name[CTA_STATS_EXP_CREATE]);
        printf("DIMENSION %s '' incremental -1 1\n", nfstat_root.attr2name[CTA_STATS_EXP_DELETE]);
        printf("DIMENSION %s '' incremental  1 1\n", nfstat_root.attr2name[CTA_STATS_EXP_NEW]);
    }

    printf(
           "BEGIN %s.%s\n"
           , RRD_TYPE_NET_STAT_NETFILTER
           , RRD_TYPE_NET_STAT_CONNTRACK "_expect"
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_EXP_CREATE]
           , (collected_number) nfstat_root.metrics[CTA_STATS_EXP_CREATE]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_EXP_DELETE]
           , (collected_number) nfstat_root.metrics[CTA_STATS_EXP_DELETE]
    );
    printf(
           "SET %s = %lld\n"
           , nfstat_root.attr2name[CTA_STATS_EXP_NEW]
           , (collected_number) nfstat_root.metrics[CTA_STATS_EXP_NEW]
    );
    printf("END\n");
}

#endif // HAVE_LINUX_NETFILTER_NFNETLINK_CONNTRACK_H


// ----------------------------------------------------------------------------
// DO_NFACCT - collect netfilter accounting statistics via netlink

#ifdef HAVE_LIBNETFILTER_ACCT
#define DO_NFACCT 1

#include <libnetfilter_acct/libnetfilter_acct.h>

struct nfacct_data {
    char *name;
    uint32_t hash;

    uint64_t pkts;
    uint64_t bytes;

    int packets_dimension_added;
    int bytes_dimension_added;

    int updated;

    struct nfacct_data *next;
};

static struct {
    int update_every;
    char *buf;
    size_t buf_size;
    struct mnl_socket *mnl;
    struct nlmsghdr *nlh;
    unsigned int seq;
    uint32_t portid;
    struct nfacct *nfacct_buffer;
    struct nfacct_data *nfacct_metrics;
} nfacct_root = {
        .update_every = 1,
        .buf = NULL,
        .buf_size = 0,
        .mnl = NULL,
        .nlh = NULL,
        .seq = 0,
        .portid = 0,
        .nfacct_buffer = NULL,
        .nfacct_metrics = NULL
};

static inline struct nfacct_data *nfacct_data_get(const char *name, uint32_t hash) {
    struct nfacct_data *d = NULL, *last = NULL;
    for(d = nfacct_root.nfacct_metrics; d ; last = d, d = d->next) {
        if(unlikely(d->hash == hash && !strcmp(d->name, name)))
            return d;
    }

    d = callocz(1, sizeof(struct nfacct_data));
    d->name = strdupz(name);
    d->hash = hash;

    if(!last) {
        d->next = nfacct_root.nfacct_metrics;
        nfacct_root.nfacct_metrics = d;
    }
    else {
        d->next = last->next;
        last->next = d;
    }

    return d;
}

static int nfacct_init(int update_every) {
    nfacct_root.update_every = update_every;

    nfacct_root.buf_size = mnl_buffer_size();
    nfacct_root.buf = mallocz(nfacct_root.buf_size);

    nfacct_root.nfacct_buffer = nfacct_alloc();
    if(!nfacct_root.nfacct_buffer) {
        error("nfacct.plugin: nfacct_alloc() failed.");
        return 0;
    }

    nfacct_root.seq = (unsigned int)now_realtime_sec() - 1;

    nfacct_root.mnl  = mnl_socket_open(NETLINK_NETFILTER);
    if(!nfacct_root.mnl) {
        error("nfacct.plugin: mnl_socket_open() failed");
        return 1;
    }

    if(mnl_socket_bind(nfacct_root.mnl, 0, MNL_SOCKET_AUTOPID) < 0) {
        error("nfacct.plugin: mnl_socket_bind() failed");
        return 1;
    }
    nfacct_root.portid = mnl_socket_get_portid(nfacct_root.mnl);

    return 0;
}

static int nfacct_callback(const struct nlmsghdr *nlh, void *data) {
    (void)data;

    if(nfacct_nlmsg_parse_payload(nlh, nfacct_root.nfacct_buffer) < 0) {
        error("NFACCT: nfacct_nlmsg_parse_payload() failed.");
        return MNL_CB_OK;
    }

    const char *name = nfacct_attr_get_str(nfacct_root.nfacct_buffer, NFACCT_ATTR_NAME);
    uint32_t hash = simple_hash(name);

    struct nfacct_data *d = nfacct_data_get(name, hash);

    d->pkts  = nfacct_attr_get_u64(nfacct_root.nfacct_buffer, NFACCT_ATTR_PKTS);
    d->bytes = nfacct_attr_get_u64(nfacct_root.nfacct_buffer, NFACCT_ATTR_BYTES);
    d->updated = 1;

    return MNL_CB_OK;
}

static int nfacct_collect() {
    // mark all old metrics as not-updated
    struct nfacct_data *d;
    for(d = nfacct_root.nfacct_metrics; d ; d = d->next)
        d->updated = 0;

    // prepare the request
    nfacct_root.seq++;
    nfacct_root.nlh = nfacct_nlmsg_build_hdr(nfacct_root.buf, NFNL_MSG_ACCT_GET, NLM_F_DUMP, (uint32_t)nfacct_root.seq);
    if(!nfacct_root.nlh) {
        error("NFACCT: nfacct_nlmsg_build_hdr() failed");
        return 1;
    }

    // send the request
    if(mnl_socket_sendto(nfacct_root.mnl, nfacct_root.nlh, nfacct_root.nlh->nlmsg_len) < 0) {
        error("NFACCT: mnl_socket_sendto() failed");
        return 1;
    }

    // get the reply
    ssize_t ret;
    while((ret = mnl_socket_recvfrom(nfacct_root.mnl, nfacct_root.buf, nfacct_root.buf_size)) > 0) {
        if(mnl_cb_run(
                nfacct_root.buf
                , (size_t)ret
                , nfacct_root.seq
                , nfacct_root.portid
                , nfacct_callback
                , NULL
        ) <= 0)
            break;
    }

    // verify we run without issues
    if (ret == -1) {
        error("NFACCT: error communicating with kernel. This plugin can only work when netdata runs as root.");
        return 1;
    }

    return 0;
}

static void nfacct_send_metrics() {
    static int bytes_chart_generated = 0, packets_chart_generated = 0;

    if(!nfacct_root.nfacct_metrics) return;
    struct nfacct_data *d;

    if(!packets_chart_generated) {
        packets_chart_generated = 1;
        printf("CHART netfilter.nfacct_packets '' 'Netfilter Accounting Packets' 'packets/s' 'nfacct' '' stacked %d %d %s\n"
               , NETDATA_CHART_PRIO_NETFILTER_PACKETS
               , nfacct_root.update_every
               , PLUGIN_NFACCT_NAME
        );
    }

    for(d = nfacct_root.nfacct_metrics; d ; d = d->next) {
        if(likely(d->updated)) {
            if(unlikely(!d->packets_dimension_added)) {
                d->packets_dimension_added = 1;
                printf("CHART netfilter.nfacct_packets '' 'Netfilter Accounting Packets' 'packets/s'\n");
                printf("DIMENSION %s '' incremental 1 %d\n", d->name, nfacct_root.update_every);
            }
            printf(
                    "BEGIN netfilter.nfacct_packets\n"
                    "SET %s = %lld\n"
                    "END\n"
                    , d->name
                    , (collected_number)d->pkts
            );
        }
    }

    // ----------------------------------------------------------------

    if(!bytes_chart_generated) {
        bytes_chart_generated = 1;
        printf("CHART netfilter.nfacct_bytes '' 'Netfilter Accounting Bandwidth' 'kilobytes/s' 'nfacct' '' stacked %d %d %s\n"
               , NETDATA_CHART_PRIO_NETFILTER_BYTES
               , nfacct_root.update_every
               , PLUGIN_NFACCT_NAME
        );
    }

    for(d = nfacct_root.nfacct_metrics; d ; d = d->next) {
        if(likely(d->updated)) {
            if(unlikely(!d->bytes_dimension_added)) {
                d->bytes_dimension_added = 1;
                printf("CHART netfilter.nfacct_bytes '' 'Netfilter Accounting Bandwidth' 'kilobytes/s'\n");
                printf("DIMENSION %s '' incremental 1 %d\n", d->name, 1000 * nfacct_root.update_every);
            }
            printf(
                   "BEGIN netfilter.nfacct_bytes\n"
                   "SET %s = %lld\n"
                   "END\n"
                   , d->name
                   , (collected_number)d->bytes
            );
        }
    }
}

#endif // HAVE_LIBNETFILTER_ACCT

int main(int argc, char **argv) {

    // ------------------------------------------------------------------------
    // initialization of netdata plugin

    program_name = "nfacct.plugin";

    // disable syslog
    error_log_syslog = 0;

    // set errors flood protection to 100 logs per hour
    error_log_errors_per_period = 100;
    error_log_throttle_period = 3600;

    // ------------------------------------------------------------------------
    // parse command line parameters

    int i, freq = 0;
    for(i = 1; i < argc ; i++) {
        if(isdigit(*argv[i]) && !freq) {
            int n = str2i(argv[i]);
            if(n > 0 && n < 86400) {
                freq = n;
                continue;
            }
        }
        else if(strcmp("version", argv[i]) == 0 || strcmp("-version", argv[i]) == 0 || strcmp("--version", argv[i]) == 0 || strcmp("-v", argv[i]) == 0 || strcmp("-V", argv[i]) == 0) {
            printf("nfacct.plugin %s\n", VERSION);
            exit(0);
        }
        else if(strcmp("debug", argv[i]) == 0) {
            debug = 1;
            continue;
        }
        else if(strcmp("-h", argv[i]) == 0 || strcmp("--help", argv[i]) == 0) {
            fprintf(stderr,
                    "\n"
                    " netdata nfacct.plugin %s\n"
                    " Copyright (C) 2015-2017 Costa Tsaousis <costa@tsaousis.gr>\n"
                    " Released under GNU General Public License v3 or later.\n"
                    " All rights reserved.\n"
                    "\n"
                    " This program is a data collector plugin for netdata.\n"
                    "\n"
                    " Available command line options:\n"
                    "\n"
                    "  COLLECTION_FREQUENCY    data collection frequency in seconds\n"
                    "                          minimum: %d\n"
                    "\n"
                    "  debug                   enable verbose output\n"
                    "                          default: disabled\n"
                    "\n"
                    "  -v\n"
                    "  -V\n"
                    "  --version               print version and exit\n"
                    "\n"
                    "  -h\n"
                    "  --help                  print this message and exit\n"
                    "\n"
                    " For more information:\n"
                    " https://github.com/netdata/netdata/tree/master/collectors/nfacct.plugin\n"
                    "\n"
                    , VERSION
                    , netdata_update_every
            );
            exit(1);
        }

        error("nfacct.plugin: ignoring parameter '%s'", argv[i]);
    }

    errno = 0;

    if(freq >= netdata_update_every)
        netdata_update_every = freq;
    else if(freq)
        error("update frequency %d seconds is too small for NFACCT. Using %d.", freq, netdata_update_every);

#ifdef DO_NFACCT
    if(debug) fprintf(stderr, "freeipmi.plugin: calling nfacct_init()\n");
    int nfacct = !nfacct_init(netdata_update_every);
#endif

#ifdef DO_NFSTAT
    if(debug) fprintf(stderr, "freeipmi.plugin: calling nfstat_init()\n");
    int nfstat = !nfstat_init(netdata_update_every);
#endif

    // ------------------------------------------------------------------------
    // the main loop

    if(debug) fprintf(stderr, "nfacct.plugin: starting data collection\n");

    time_t started_t = now_monotonic_sec();

    size_t iteration;
    usec_t step = netdata_update_every * USEC_PER_SEC;

    heartbeat_t hb;
    heartbeat_init(&hb);
    for(iteration = 0; 1; iteration++) {
        usec_t dt = heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        if(debug && iteration)
            fprintf(stderr, "nfacct.plugin: iteration %zu, dt %llu usec\n"
                    , iteration
                    , dt
            );

#ifdef DO_NFACCT
        if(likely(nfacct)) {
            if(debug) fprintf(stderr, "nfacct.plugin: calling nfacct_collect()\n");
            nfacct = !nfacct_collect();

            if(likely(nfacct)) {
                if(debug) fprintf(stderr, "nfacct.plugin: calling nfacct_send_metrics()\n");
                nfacct_send_metrics();
            }
        }
#endif

#ifdef DO_NFSTAT
        if(likely(nfstat)) {
            if(debug) fprintf(stderr, "nfacct.plugin: calling nfstat_collect()\n");
            nfstat = !nfstat_collect();

            if(likely(nfstat)) {
                if(debug) fprintf(stderr, "nfacct.plugin: calling nfstat_send_metrics()\n");
                nfstat_send_metrics();
            }
        }
#endif

        fflush(stdout);

        // restart check (14400 seconds)
        if(now_monotonic_sec() - started_t > 14400) break;
    }

    info("NFACCT process exiting");
}

#else // !HAVE_LIBMNL

int main(int argc, char **argv) {
    fatal("nfacct.plugin is not compiled.");
}

#endif // !HAVE_LIBMNL
