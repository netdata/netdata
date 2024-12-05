// SPDX-License-Identifier: GPL-3.0-or-later

#include "commands.h"
#include "plugins.d/pluginsd_internals.h"

PARSER_RC stream_receiver_pluginsd_claimed_id(char **words, size_t num_words, PARSER *parser) {
    const char *machine_guid_str = get_word(words, num_words, 1);
    const char *claim_id_str = get_word(words, num_words, 2);

    if (!machine_guid_str || !claim_id_str) {
        netdata_log_error("PLUGINSD: command CLAIMED_ID came malformed, machine_guid '%s', claim_id '%s'",
                          machine_guid_str ? machine_guid_str : "[unset]",
                          claim_id_str ? claim_id_str : "[unset]");
        return PARSER_RC_ERROR;
    }

    RRDHOST *host = parser->user.host;

    nd_uuid_t machine_uuid;
    if(uuid_parse(machine_guid_str, machine_uuid)) {
        netdata_log_error("PLUGINSD: parameter machine guid to CLAIMED_ID command is not valid UUID. "
                          "Received: '%s'.", machine_guid_str);
        return PARSER_RC_ERROR;
    }

    nd_uuid_t claim_uuid;
    if(strcmp(claim_id_str, "NULL") == 0)
        uuid_clear(claim_uuid);

    else if(uuid_parse(claim_id_str, claim_uuid) != 0) {
        netdata_log_error("PLUGINSD: parameter claim id to CLAIMED_ID command is not valid UUID. "
                          "Received: '%s'.", claim_id_str);
        return PARSER_RC_ERROR;
    }

    if(strcmp(machine_guid_str, host->machine_guid) != 0) {
        netdata_log_error("PLUGINSD: received claim id for host '%s' but it came over the connection of '%s'",
                          machine_guid_str, host->machine_guid);
        return PARSER_RC_OK; //the message is OK problem must be somewhere else
    }

    if(host == localhost) {
        netdata_log_error("PLUGINSD: CLAIMED_ID command cannot be used to set the claimed id of localhost. "
                          "Received: '%s'.", claim_id_str);
        return PARSER_RC_OK;
    }

    if(!uuid_is_null(claim_uuid)) {
        uuid_copy(host->aclk.claim_id_of_origin.uuid, claim_uuid);
        stream_sender_send_claimed_id(host);
    }

    return PARSER_RC_OK;
}

void stream_sender_send_claimed_id(RRDHOST *host) {
    if(!stream_sender_has_capabilities(host, STREAM_CAP_CLAIM))
        return;

    if(unlikely(!rrdhost_can_stream_metadata_to_parent(host)))
        return;

    CLEAN_BUFFER *wb = buffer_create(0, NULL);

    char str[UUID_STR_LEN] = "";
    ND_UUID uuid = host->aclk.claim_id_of_origin;
    if(!UUIDiszero(uuid))
        uuid_unparse_lower(uuid.uuid, str);
    else
        strncpyz(str, "NULL", sizeof(str) - 1);

    buffer_sprintf(wb, PLUGINSD_KEYWORD_CLAIMED_ID " '%s' '%s'\n",
                   host->machine_guid, str);

    sender_commit_clean_buffer(host->sender, wb, STREAM_TRAFFIC_TYPE_METADATA);
}
