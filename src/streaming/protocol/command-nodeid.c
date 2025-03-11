// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "../stream-receiver-internals.h"
#include "../stream-sender-internals.h"
#include "plugins.d/pluginsd_internals.h"

// the child disconnected from the parent, and it has to clear the parent's claim id
void stream_sender_clear_parent_claim_id(RRDHOST *host) {
    if (!UUIDiszero(host->aclk.claim_id_of_parent)) {
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "Host '%s' [PCLAIMID] cleared parent's claim id",
               rrdhost_hostname(host));

        host->aclk.claim_id_of_parent = UUID_ZERO;
    }
}

// the parent sends to the child its claim id, node id and cloud url
void stream_receiver_send_node_and_claim_id_to_child(RRDHOST *host) {
    if(rrdhost_is_local(host) || UUIDiszero(host->node_id)) return;

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
void stream_sender_get_node_and_claim_id_from_parent(struct sender_state *s, const char *claim_id_str, const char *node_id_str, const char *url) {

    bool claimed = is_agent_claimed();
    bool update_node_id = false;

    // ----------------------------------------------------------------------------------------------------------------
    // validate the parameters

    ND_UUID claim_id;
    if (uuid_parse(claim_id_str ? claim_id_str : "", claim_id.uuid) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND '%s' [to %s] [PCLAIMID]: received invalid claim id '%s'",
               rrdhost_hostname(s->host), s->remote_ip,
               claim_id_str ? claim_id_str : "(unset)");
        return;
    }

    if(UUIDiszero(claim_id)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM SND '%s' [to %s] [PCLAIMID]: received zero claim id '%s'",
               rrdhost_hostname(s->host), s->remote_ip,
               claim_id_str ? claim_id_str : "(unset)");
        return;
    }

    ND_UUID node_id;
    if(uuid_parse(node_id_str ? node_id_str : "", node_id.uuid) != 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND '%s' [to %s] [PCLAIMID] received an invalid node id '%s'",
               rrdhost_hostname(s->host), s->remote_ip,
               node_id_str ? node_id_str : "(unset)");
        return;
    }

    if(UUIDiszero(node_id)) {
        nd_log(NDLS_DAEMON, NDLP_DEBUG,
               "STREAM SND '%s' [to %s] [PCLAIMID]: received zero node id '%s'",
               rrdhost_hostname(s->host), s->remote_ip,
               node_id_str ? node_id_str : "(unset)");
        return;
    }

    if(!url || !*url) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "STREAM SND '%s' [to %s] [PCLAIMID] received an invalid cloud URL '%s'",
               rrdhost_hostname(s->host), s->remote_ip,
               url ? url : "(unset)");
        return;
    }

    // ----------------------------------------------------------------------------------------------------------------
    // the parameters are ok
    // apply the changes

    if (!UUIDeq(s->host->aclk.claim_id_of_parent, claim_id)) {
        if(UUIDiszero(s->host->aclk.claim_id_of_parent))
            nd_log(NDLS_DAEMON, NDLP_INFO,
                   "STREAM SND '%s' [to %s] [PCLAIMID] set parent's claim id to %s (was empty)",
                   rrdhost_hostname(s->host), s->remote_ip,
                   claim_id_str ? claim_id_str : "(unset)");
        else
            nd_log(NDLS_DAEMON, NDLP_INFO,
                   "STREAM SND '%s' [to %s] [PCLAIMID] changed parent's claim id to %s (was set)",
                   rrdhost_hostname(s->host), s->remote_ip,
                   claim_id_str ? claim_id_str : "(unset)");

        s->host->aclk.claim_id_of_parent = claim_id;
    }

    if(!UUIDiszero(s->host->node_id) && !UUIDeq(s->host->node_id, node_id)) {
        if(claimed) {
            update_node_id = false;
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "STREAM SND '%s' [to %s] [PCLAIMID] parent reports different node id '%s', but we are claimed. Ignoring it.",
                   rrdhost_hostname(s->host), s->remote_ip,
                   node_id_str ? node_id_str : "(unset)");
        }
        else {
            update_node_id = true;
            nd_log(NDLS_DAEMON, NDLP_WARNING,
                   "STREAM SND '%s' [to %s] [PCLAIMID] changed node id to %s",
                   rrdhost_hostname(s->host), s->remote_ip,
                   node_id_str ? node_id_str : "(unset)");
        }
    }

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
