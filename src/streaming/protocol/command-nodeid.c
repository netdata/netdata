// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "collectors/plugins.d/pluginsd_internals.h"

void rrdpush_send_child_node_id(RRDHOST *host) {
    if(host == localhost) return;

    spinlock_lock(&host->receiver_lock);
    if(host->receiver && stream_has_capability(host->receiver, STREAM_CAP_NODE_ID)) {
        char node_id_str[UUID_STR_LEN];
        uuid_unparse_lower(host->node_id, node_id_str);

        char buf[100];
        snprintfz(buf, sizeof(buf), PLUGINSD_KEYWORD_NODE_ID " '%s' '%s'",
                  node_id_str, cloud_config_url_get());

        send_to_plugin(buf, host->receiver);
    }
    spinlock_unlock(&host->receiver_lock);
}

void streaming_sender_command_node_id_parser(struct sender_state *s) {
    if(!aclk_connected) {
        nd_uuid_t node_uuid, claim_uuid;

        char *claim_id = get_word(s->line.words, s->line.num_words, 1);
        char *node_id = get_word(s->line.words, s->line.num_words, 2);
        char *url = get_word(s->line.words, s->line.num_words, 3);

        bool doit = true;
        if (uuid_parse(claim_id ? claim_id : "", claim_uuid) != 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM %s [send to %s] received invalid claim id '%s'",
                   rrdhost_hostname(s->host), s->connected_to,
                   claim_id ? claim_id : "(unset)");
            doit = false;
        }

        if(uuid_parse(node_id ? node_id : "", node_uuid) != 0 || (!uuid_is_null(s->host->node_id) && !uuid_eq(node_uuid, s->host->node_id))) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "STREAM %s [send to %s] received an invalid/different node id '%s'",
                   rrdhost_hostname(s->host), s->connected_to,
                   node_id ? node_id : "(unset)");
            doit = false;
        }

        if(doit) {
            if (!uuid_is_null(s->host->aclk.claim_id_of_parent) && !uuid_eq(s->host->aclk.claim_id_of_parent, claim_uuid))
                nd_log(NDLS_DAEMON, NDLP_INFO,
                       "STREAM %s [send to %s] changed parent's claim id to %s",
                       rrdhost_hostname(s->host), s->connected_to, claim_id ? claim_id : "(unset)");

            uuid_copy(s->host->node_id, node_uuid);
            uuid_copy(s->host->aclk.claim_id_of_parent, claim_uuid);

            if(!is_agent_claimed())
                cloud_config_url_set(url);

            // send it down the line (to children)
            rrdpush_send_child_node_id(s->host);
        }
    }
}
