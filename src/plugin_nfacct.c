#include "common.h"

#ifdef INTERNAL_PLUGIN_NFACCT
#include <libmnl/libmnl.h>
#include <libnetfilter_acct/libnetfilter_acct.h>

struct nfacct_data {
    char *name;
    uint32_t hash;

    uint64_t pkts;
    uint64_t bytes;

    RRDDIM *rd_bytes;
    RRDDIM *rd_packets;

    int updated;

    struct nfacct_data *next;
};

static struct nfacct_list {
    struct nfacct *nfacct_buffer;
    struct nfacct_data *nfacct_metrics;
} nfacct_root = {
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

static int nfacct_callback(const struct nlmsghdr *nlh, void *data) {
    (void)data;

    if(nfacct_nlmsg_parse_payload(nlh, nfacct_root.nfacct_buffer) < 0) {
        error("nfacct.plugin: nfacct_nlmsg_parse_payload() failed.");
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

void *nfacct_main(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;

    info("NFACCT thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("nfacct.plugin: Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("nfacct.plugin: Cannot set pthread cancel state to ENABLE.");

    nfacct_root.nfacct_buffer = nfacct_alloc();
    if(!nfacct_root.nfacct_buffer)
        fatal("nfacct.plugin: nfacct_alloc() failed.");

    char buf[MNL_SOCKET_BUFFER_SIZE];
    struct mnl_socket *nl = NULL;
    struct nlmsghdr *nlh = NULL;
    unsigned int seq = 0, portid = 0;

    seq = (unsigned int)now_realtime_sec() - 1;

    nl  = mnl_socket_open(NETLINK_NETFILTER);
    if(!nl) {
        error("nfacct.plugin: mnl_socket_open() failed");
        goto cleanup;
    }

    if(mnl_socket_bind(nl, 0, MNL_SOCKET_AUTOPID) < 0) {
        error("nfacct.plugin: mnl_socket_bind() failed");
        goto cleanup;
    }
    portid = mnl_socket_get_portid(nl);

    // ------------------------------------------------------------------------

    RRDSET *st_bytes = NULL, *st_packets = NULL;

    // ------------------------------------------------------------------------

    int update_every = (int)config_get_number("plugin:nfacct", "update every", localhost->rrd_update_every);
    if(update_every < localhost->rrd_update_every)
        update_every = localhost->rrd_update_every;

    usec_t step = update_every * USEC_PER_SEC;
    heartbeat_t hb;
    heartbeat_init(&hb);
    for(;;) {
        heartbeat_dt_usec(&hb);
        heartbeat_next(&hb, step);

        if(unlikely(netdata_exit)) break;

        seq++;

        nlh = nfacct_nlmsg_build_hdr(buf, NFNL_MSG_ACCT_GET, NLM_F_DUMP, (uint32_t)seq);
        if(!nlh) {
            error("nfacct.plugin: nfacct_nlmsg_build_hdr() failed");
            goto cleanup;
        }

        if(mnl_socket_sendto(nl, nlh, nlh->nlmsg_len) < 0) {
            error("nfacct.plugin: mnl_socket_send");
            goto cleanup;
        }

        ssize_t ret;
        while((ret = mnl_socket_recvfrom(nl, buf, sizeof(buf))) > 0) {
            if((ret = mnl_cb_run(buf, (size_t)ret, seq, portid, nfacct_callback, NULL)) <= 0) break;
        }

        if (ret == -1) {
            error("nfacct.plugin: error communicating with kernel. NFACCT plugin can only work when netdata runs as root.");
            goto cleanup;
        }

        // --------------------------------------------------------------------

        if(nfacct_root.nfacct_metrics) {
            struct nfacct_data *d;

            if(!st_packets) {
                st_packets = rrdset_create_localhost(
                        "netfilter"
                        , "nfacct_packets"
                        , NULL
                        , "nfacct"
                        , NULL
                        , "Netfilter Accounting Packets"
                        , "packets/s"
                        , 3206
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
            }
            else rrdset_next(st_packets);

            for(d = nfacct_root.nfacct_metrics; d ; d = d->next) {
                if(likely(d->updated)) {
                    if(unlikely(!d->rd_packets))
                        d->rd_packets = rrddim_add(
                                st_packets
                                , d->name
                                , NULL
                                , 1
                                , update_every
                                , RRD_ALGORITHM_INCREMENTAL
                        );

                    rrddim_set_by_pointer(
                            st_packets
                            , d->rd_packets
                            , (collected_number)d->pkts
                    );
                }
            }

            rrdset_done(st_packets);

            // ----------------------------------------------------------------

            st_bytes = rrdset_find_bytype_localhost("netfilter", "nfacct_bytes");
            if(!st_bytes) {
                st_bytes = rrdset_create_localhost(
                        "netfilter"
                        , "nfacct_bytes"
                        , NULL
                        , "nfacct"
                        , NULL
                        , "Netfilter Accounting Bandwidth"
                        , "kilobytes/s"
                        , 3207
                        , update_every
                        , RRDSET_TYPE_STACKED
                );
            }
            else rrdset_next(st_bytes);

            for(d = nfacct_root.nfacct_metrics; d ; d = d->next) {
                if(likely(d->updated)) {
                    if(unlikely(!d->rd_bytes))
                        d->rd_bytes = rrddim_add(
                                st_bytes
                                , d->name
                                , NULL
                                , 1
                                , 1000 * update_every
                                , RRD_ALGORITHM_INCREMENTAL
                        );

                    rrddim_set_by_pointer(
                            st_bytes
                            , d->rd_bytes
                            , (collected_number)d->bytes
                    );
                }
            }

            rrdset_done(st_bytes);


            // ----------------------------------------------------------------
            // prepare for the next loop

            for(d = nfacct_root.nfacct_metrics; d ; d = d->next)
                d->updated = 0;
        }
    }

cleanup:
    info("NFACCT thread exiting");

    if(nl) mnl_socket_close(nl);

    static_thread->enabled = 0;
    pthread_exit(NULL);
    return NULL;
}
#endif
