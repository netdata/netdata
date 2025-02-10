// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

const char *cloud_status_to_string(CLOUD_STATUS status) {
    switch(status) {
        default:
        case CLOUD_STATUS_AVAILABLE:
            return "available";

        case CLOUD_STATUS_BANNED:
            return "banned";

        case CLOUD_STATUS_OFFLINE:
            return "offline";

        case CLOUD_STATUS_ONLINE:
            return "online";

        case CLOUD_STATUS_CONNECTING:
            return "connecting";

        case CLOUD_STATUS_INDIRECT:
            return "indirect";
    }
}

CLOUD_STATUS cloud_status(void) {
    if(unlikely(aclk_disable_runtime))
        return CLOUD_STATUS_BANNED;

    if(likely(aclk_online())) {
        if (rrdhost_flag_check(localhost, RRDHOST_FLAG_ACLK_STREAM_CONTEXTS))
            return CLOUD_STATUS_ONLINE;
        else
            return CLOUD_STATUS_CONNECTING;
    }

    if(localhost->sender &&
        rrdhost_flag_check(localhost, RRDHOST_FLAG_STREAM_SENDER_READY_4_METRICS) &&
        stream_sender_has_capabilities(localhost, STREAM_CAP_NODE_ID) &&
        !UUIDiszero(localhost->node_id) &&
        !UUIDiszero(localhost->aclk.claim_id_of_parent))
        return CLOUD_STATUS_INDIRECT;

    if(is_agent_claimed())
        return CLOUD_STATUS_OFFLINE;

    return CLOUD_STATUS_AVAILABLE;
}

time_t cloud_last_change(void) {
    time_t ret = MAX(last_conn_time_mqtt, last_disconnect_time);
    if(!ret) ret = netdata_start_time;
    return ret;
}

time_t cloud_next_connection_attempt(void) {
    return next_connection_attempt;
}

size_t cloud_connection_id(void) {
    return aclk_connection_counter;
}

const char *cloud_status_aclk_offline_reason() {
    if(aclk_disable_runtime)
        return "banned";

    return aclk_status_to_string();
}

const char *cloud_status_aclk_base_url() {
    return aclk_cloud_base_url;
}

CLOUD_STATUS buffer_json_cloud_status(BUFFER *wb, time_t now_s) {
    CLOUD_STATUS status = cloud_status();

    buffer_json_member_add_object(wb, "cloud");
    {
        size_t id = cloud_connection_id();
        time_t last_change = cloud_last_change();
        time_t next_connect = cloud_next_connection_attempt();
        buffer_json_member_add_uint64(wb, "id", id);
        buffer_json_member_add_string(wb, "status", cloud_status_to_string(status));
        buffer_json_member_add_time_t(wb, "since", last_change);
        buffer_json_member_add_time_t(wb, "age", now_s - last_change);

        switch(status) {
            default:
            case CLOUD_STATUS_AVAILABLE:
                // the agent is not claimed
                buffer_json_member_add_string(wb, "url", cloud_config_url_get());
                buffer_json_member_add_string(wb, "reason", claim_agent_failure_reason_get());
                break;

            case CLOUD_STATUS_BANNED: {
                // the agent is claimed, but has been banned from NC
                CLAIM_ID claim_id = claim_id_get();
                buffer_json_member_add_string(wb, "claim_id", claim_id.str);
                buffer_json_member_add_string(wb, "url", cloud_status_aclk_base_url());
                buffer_json_member_add_string(wb, "reason", "Agent is banned from Netdata Cloud");
                buffer_json_member_add_string(wb, "url", cloud_config_url_get());
                break;
            }

            case CLOUD_STATUS_OFFLINE: {
                // the agent is claimed, but cannot get online
                CLAIM_ID claim_id = rrdhost_claim_id_get(localhost);
                buffer_json_member_add_string(wb, "claim_id", claim_id.str);
                buffer_json_member_add_string(wb, "url", cloud_status_aclk_base_url());
                buffer_json_member_add_string(wb, "reason", cloud_status_aclk_offline_reason());
                if (next_connect > now_s) {
                    buffer_json_member_add_time_t(wb, "next_check", next_connect);
                    buffer_json_member_add_time_t(wb, "next_in", next_connect - now_s);
                }
                break;
            }

            case CLOUD_STATUS_ONLINE: {
                // the agent is claimed and online
                CLAIM_ID claim_id = claim_id_get();
                buffer_json_member_add_string(wb, "claim_id", claim_id.str);
                buffer_json_member_add_string(wb, "url", cloud_status_aclk_base_url());
                buffer_json_member_add_string(wb, "reason", "");
                break;
            }

            case CLOUD_STATUS_INDIRECT: {
                CLAIM_ID claim_id = rrdhost_claim_id_get(localhost);
                buffer_json_member_add_string(wb, "claim_id", claim_id.str);
                buffer_json_member_add_string(wb, "url", cloud_config_url_get());
                buffer_json_member_add_string(wb, "reason", cloud_status_aclk_offline_reason());
                break;
            }
        }
    }
    buffer_json_object_close(wb); // cloud

    return status;
}
