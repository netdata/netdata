#include "common.h"

#ifdef INTERNAL_PLUGIN_NFACCT
#include <libmnl/libmnl.h>
#include <libnetfilter_acct/libnetfilter_acct.h>

struct mynfacct {
    const char *name;
    uint64_t pkts;
    uint64_t bytes;
    struct nfacct *nfacct;
};

struct nfacct_list {
    int size;
    int len;
    struct mynfacct data[];
} *nfacct_list = NULL;

static int nfacct_callback(const struct nlmsghdr *nlh, void *data) {
    if(data) {};

    if(!nfacct_list || nfacct_list->len == nfacct_list->size) {
        int size = (nfacct_list) ? nfacct_list->size : 0;
        int len = (nfacct_list) ? nfacct_list->len : 0;
        size++;

        info("nfacct.plugin: increasing nfacct_list to size %d", size);

        nfacct_list = reallocz(nfacct_list, sizeof(struct nfacct_list) + (sizeof(struct mynfacct) * size));

        nfacct_list->data[len].nfacct = nfacct_alloc();
        if(!nfacct_list->data[size - 1].nfacct) {
            error("nfacct.plugin: nfacct_alloc() failed.");
            free(nfacct_list);
            nfacct_list = NULL;
            return MNL_CB_OK;
        }

        nfacct_list->size = size;
        nfacct_list->len = len;
    }

    if(nfacct_nlmsg_parse_payload(nlh, nfacct_list->data[nfacct_list->len].nfacct) < 0) {
        error("nfacct.plugin: nfacct_nlmsg_parse_payload() failed.");
        return MNL_CB_OK;
    }

    nfacct_list->data[nfacct_list->len].name  = nfacct_attr_get_str(nfacct_list->data[nfacct_list->len].nfacct, NFACCT_ATTR_NAME);
    nfacct_list->data[nfacct_list->len].pkts  = nfacct_attr_get_u64(nfacct_list->data[nfacct_list->len].nfacct, NFACCT_ATTR_PKTS);
    nfacct_list->data[nfacct_list->len].bytes = nfacct_attr_get_u64(nfacct_list->data[nfacct_list->len].nfacct, NFACCT_ATTR_BYTES);

    nfacct_list->len++;
    return MNL_CB_OK;
}

void *nfacct_main(void *ptr) {
    if(ptr) { ; }

    info("NFACCT thread created with task id %d", gettid());

    if(pthread_setcanceltype(PTHREAD_CANCEL_DEFERRED, NULL) != 0)
        error("nfacct.plugin: Cannot set pthread cancel type to DEFERRED.");

    if(pthread_setcancelstate(PTHREAD_CANCEL_ENABLE, NULL) != 0)
        error("nfacct.plugin: Cannot set pthread cancel state to ENABLE.");

    char buf[MNL_SOCKET_BUFFER_SIZE];
    struct mnl_socket *nl = NULL;
    struct nlmsghdr *nlh = NULL;
    unsigned int seq = 0, portid = 0;

    seq = now_realtime_sec() - 1;

    nl  = mnl_socket_open(NETLINK_NETFILTER);
    if(!nl) {
        error("nfacct.plugin: mnl_socket_open() failed");
        pthread_exit(NULL);
        return NULL;
    }

    if(mnl_socket_bind(nl, 0, MNL_SOCKET_AUTOPID) < 0) {
        mnl_socket_close(nl);
        error("nfacct.plugin: mnl_socket_bind() failed");
        pthread_exit(NULL);
        return NULL;
    }
    portid = mnl_socket_get_portid(nl);

    // ------------------------------------------------------------------------

    struct timeval last, now;
    usec_t usec = 0, susec = 0;
    RRDSET *st = NULL;

    now_realtime_timeval(&last);

    // ------------------------------------------------------------------------

    while(1) {
        if(unlikely(netdata_exit)) break;

        seq++;

        nlh = nfacct_nlmsg_build_hdr(buf, NFNL_MSG_ACCT_GET, NLM_F_DUMP, seq);
        if(!nlh) {
            mnl_socket_close(nl);
            error("nfacct.plugin: nfacct_nlmsg_build_hdr() failed");
            pthread_exit(NULL);
            return NULL;
        }

        if(mnl_socket_sendto(nl, nlh, nlh->nlmsg_len) < 0) {
            error("nfacct.plugin: mnl_socket_send");
            pthread_exit(NULL);
            return NULL;
        }

        if(nfacct_list) nfacct_list->len = 0;

        int ret;
        while((ret = mnl_socket_recvfrom(nl, buf, sizeof(buf))) > 0) {
            if((ret = mnl_cb_run(buf, ret, seq, portid, nfacct_callback, NULL)) <= 0) break;
        }

        if (ret == -1) {
            error("nfacct.plugin: error communicating with kernel.");
            pthread_exit(NULL);
            return NULL;
        }

        // --------------------------------------------------------------------

        now_realtime_timeval(&now);
        usec = dt_usec(&now, &last) - susec;
        debug(D_NFACCT_LOOP, "nfacct.plugin: last loop took %llu usec (worked for %llu, sleeped for %llu).", usec + susec, usec, susec);

        if(usec < (rrd_update_every * 1000000ULL / 2ULL)) susec = (rrd_update_every * 1000000ULL) - usec;
        else susec = rrd_update_every * 1000000ULL / 2ULL;


        // --------------------------------------------------------------------

        if(nfacct_list && nfacct_list->len) {
            int i;

            st = rrdset_find_bytype("netfilter", "nfacct_packets");
            if(!st) {
                st = rrdset_create("netfilter", "nfacct_packets", NULL, "nfacct", NULL, "Netfilter Accounting Packets", "packets/s", 3206, rrd_update_every, RRDSET_TYPE_STACKED);

                for(i = 0; i < nfacct_list->len ; i++)
                    rrddim_add(st, nfacct_list->data[i].name, NULL, 1, rrd_update_every, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            for(i = 0; i < nfacct_list->len ; i++) {
                RRDDIM *rd = rrddim_find(st, nfacct_list->data[i].name);

                if(!rd) rd = rrddim_add(st, nfacct_list->data[i].name, NULL, 1, rrd_update_every, RRDDIM_INCREMENTAL);
                if(rd) rrddim_set_by_pointer(st, rd, nfacct_list->data[i].pkts);
            }

            rrdset_done(st);

            // ----------------------------------------------------------------

            st = rrdset_find_bytype("netfilter", "nfacct_bytes");
            if(!st) {
                st = rrdset_create("netfilter", "nfacct_bytes", NULL, "nfacct", NULL, "Netfilter Accounting Bandwidth", "kilobytes/s", 3207, rrd_update_every, RRDSET_TYPE_STACKED);

                for(i = 0; i < nfacct_list->len ; i++)
                    rrddim_add(st, nfacct_list->data[i].name, NULL, 1, 1000 * rrd_update_every, RRDDIM_INCREMENTAL);
            }
            else rrdset_next(st);

            for(i = 0; i < nfacct_list->len ; i++) {
                RRDDIM *rd = rrddim_find(st, nfacct_list->data[i].name);

                if(!rd) rd = rrddim_add(st, nfacct_list->data[i].name, NULL, 1, 1000 * rrd_update_every, RRDDIM_INCREMENTAL);
                if(rd) rrddim_set_by_pointer(st, rd, nfacct_list->data[i].bytes);
            }

            rrdset_done(st);
        }

        // --------------------------------------------------------------------

        usleep(susec);

        // copy current to last
        memmove(&last, &now, sizeof(struct timeval));
    }

    mnl_socket_close(nl);
    pthread_exit(NULL);
    return NULL;
}
#endif
