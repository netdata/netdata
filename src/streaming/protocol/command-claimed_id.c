// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "collectors/plugins.d/pluginsd_internals.h"

PARSER_RC rrdpush_receiver_pluginsd_claimed_id(char **words, size_t num_words, PARSER *parser) {
    const char *machine_guid_str = get_word(words, num_words, 1);
    const char *claim_id_str = get_word(words, num_words, 2);

    if (!machine_guid_str || !claim_id_str) {
        netdata_log_error("PLUGINSD: command CLAIMED_ID came malformed, machine_guid '%s', claim_id '%s'",
                          machine_guid_str ? machine_guid_str : "[unset]",
                          claim_id_str ? claim_id_str : "[unset]");
        return PARSER_RC_ERROR;
    }

    nd_uuid_t uuid;
    RRDHOST *host = parser->user.host;

    // We don't need the parsed UUID
    // just do it to check the format
    if(uuid_parse(machine_guid_str, uuid)) {
        netdata_log_error("PLUGINSD: parameter machine guid to CLAIMED_ID command is not valid UUID. "
                          "Received: '%s'.", machine_guid_str);
        return PARSER_RC_ERROR;
    }

    if(uuid_parse(claim_id_str, uuid) && strcmp(claim_id_str, "NULL") != 0) {
        netdata_log_error("PLUGINSD: parameter claim id to CLAIMED_ID command is not valid UUID. "
                          "Received: '%s'.", claim_id_str);
        return PARSER_RC_ERROR;
    }

    if(strcmp(machine_guid_str, host->machine_guid) != 0) {
        netdata_log_error("PLUGINSD: received claim id for host '%s' but it came over the connection of '%s'",
                          machine_guid_str, host->machine_guid);
        return PARSER_RC_OK; //the message is OK problem must be somewhere else
    }

    rrdhost_aclk_state_lock(host);

    if (host->aclk_state.claimed_id)
        freez(host->aclk_state.claimed_id);

    host->aclk_state.claimed_id = strcmp(claim_id_str, "NULL") ? strdupz(claim_id_str) : NULL;

    rrdhost_aclk_state_unlock(host);

    rrdhost_flag_set(host, RRDHOST_FLAG_METADATA_CLAIMID | RRDHOST_FLAG_METADATA_UPDATE);

    rrdpush_sender_send_claimed_id(host);

    return PARSER_RC_OK;
}

void rrdpush_sender_send_claimed_id(RRDHOST *host) {
    if(!stream_has_capability(host->sender, STREAM_CAP_CLAIM))
        return;

    if(unlikely(!rrdhost_can_send_definitions_to_parent(host)))
        return;

    BUFFER *wb = sender_start(host->sender);
    rrdhost_aclk_state_lock(host);

    buffer_sprintf(wb, PLUGINSD_KEYWORD_CLAIMED_ID " '%s' '%s'\n",
                   host->machine_guid,
                   (host->aclk_state.claimed_id ? host->aclk_state.claimed_id : "NULL") );

    rrdhost_aclk_state_unlock(host);
    sender_commit(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);

    sender_thread_buffer_free();
}
