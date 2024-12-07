// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-receiver-internals.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

// the child disconnected from the parent, and it has to clear the parent's claim id
void stream_sender_clear_parent_claim_id(RRDHOST *host) {
    host->aclk.claim_id_of_parent = UUID_ZERO;
}

// the parent sends to the child its claim id, node id and cloud url
void stream_receiver_send_node_and_claim_id_to_child(RRDHOST *host) {
    if(host == localhost || UUIDiszero(host->node_id)) return;

    rrdhost_receiver_lock(host);
    if(stream_has_capability(host->receiver, STREAM_CAP_NODE_ID)) {
        char node_id_str[UUID_STR_LEN] = "";
        uuid_unparse_lower(host->node_id.uuid, node_id_str);

        CLAIM_ID claim_id = claim_id_get();

        if((!claim_id_is_set(claim_id) || !aclk_online())) {
            // the agent is not claimed or not connected, just use parent claim id
            // to allow the connection flow.
            // this may be zero and it is ok.
            claim_id.uuid = host->aclk.claim_id_of_parent;
            uuid_unparse_lower(claim_id.uuid.uuid, claim_id.str);
        }

        char buf[4096];
        snprintfz(buf, sizeof(buf),
                  PLUGINSD_KEYWORD_NODE_ID " '%s' '%s' '%s'\n",
                  claim_id.str, node_id_str, cloud_config_url_get());

        send_to_plugin(buf, __atomic_load_n(&host->receiver->thread.parser, __ATOMIC_RELAXED), STREAM_TRAFFIC_TYPE_METADATA);
    }
    rrdhost_receiver_unlock(host);
}

// the sender of the child receives node id, claim id and cloud url from the receiver of the parent
void stream_sender_get_node_and_claim_id_from_parent(struct sender_state *s) {
    char *claim_id_str = get_word(s->rbuf.line.words, s->rbuf.line.num_words, 1);
    char *node_id_str = get_word(s->rbuf.line.words, s->rbuf.line.num_words, 2);
    char *url = get_word(s->rbuf.line.words, s->rbuf.line.num_words, 3);

    bool claimed = is_agent_claimed();
    bool update_node_id = false;

    ND_UUID claim_id;
    if (uuid_parse(claim_id_str ? claim_id_str : "", claim_id.uuid) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s] received invalid claim id '%s'",
               rrdhost_hostname(s->host), s->connected_to,
               claim_id_str ? claim_id_str : "(unset)");
        return;
    }

    ND_UUID node_id;
    if(uuid_parse(node_id_str ? node_id_str : "", node_id.uuid) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s] received an invalid node id '%s'",
               rrdhost_hostname(s->host), s->connected_to,
               node_id_str ? node_id_str : "(unset)");
        return;
    }

    if (!UUIDiszero(s->host->aclk.claim_id_of_parent) && !UUIDeq(s->host->aclk.claim_id_of_parent, claim_id))
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "STREAM %s [send to %s] changed parent's claim id to %s",
               rrdhost_hostname(s->host), s->connected_to,
               claim_id_str ? claim_id_str : "(unset)");

    if(!UUIDiszero(s->host->node_id) && !UUIDeq(s->host->node_id, node_id)) {
        if(claimed) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM %s [send to %s] parent reports different node id '%s', but we are claimed. Ignoring it.",
                   rrdhost_hostname(s->host), s->connected_to,
                   node_id_str ? node_id_str : "(unset)");
            return;
        }
        else {
            update_node_id = true;
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "STREAM %s [send to %s] changed node id to %s",
                   rrdhost_hostname(s->host), s->connected_to,
                   node_id_str ? node_id_str : "(unset)");
        }
    }

    if(!url || !*url) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM %s [send to %s] received an invalid cloud URL '%s'",
               rrdhost_hostname(s->host), s->connected_to,
               url ? url : "(unset)");
        return;
    }

    s->host->aclk.claim_id_of_parent = claim_id;

    // There are some very strange corner cases here:
    //
    // - Agent is claimed but offline, and it receives node_id and cloud_url from a different Netdata Cloud.
    // - Agent is configured to talk to an on-prem Netdata Cloud, it is offline, but the parent is connected
    //   to a different Netdata Cloud.
    //
    // The solution below, tries to get the agent online, using the latest information.
    // So, if the agent is not claimed or not connected, we inherit whatever information sent from the parent,
    // to allow the user to work with it.

    if(claimed && aclk_online())
        // we are directly claimed and connected, ignore node id and cloud url
        return;

    bool node_id_updated = false;
    if(UUIDiszero(s->host->node_id) || update_node_id) {
        s->host->node_id = node_id;
        node_id_updated = true;
    }

    // we change the URL, to allow the agent dashboard to work with Netdata Cloud on-prem, if any.
    if(node_id_updated)
        cloud_config_url_set(url);

    // send it down the line (to children)
    stream_receiver_send_node_and_claim_id_to_child(s->host);

    if(node_id_updated)
        stream_path_node_id_updated(s->host);
}
