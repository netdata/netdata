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

        case CLOUD_STATUS_INDIRECT:
            return "indirect";
    }
}

CLOUD_STATUS cloud_status(void) {
    if(aclk_disable_runtime)
        return CLOUD_STATUS_BANNED;

    if(aclk_connected)
        return CLOUD_STATUS_ONLINE;

    {
        char *agent_id = aclk_get_claimed_id();
        bool claimed = agent_id != NULL;
        freez(agent_id);

        if(claimed)
            return CLOUD_STATUS_OFFLINE;
    }

    if(rrdhost_flag_check(localhost, RRDHOST_FLAG_RRDPUSH_SENDER_READY_4_METRICS) &&
        stream_has_capability(localhost->sender, STREAM_CAP_NODE_ID) &&
        !uuid_is_null(localhost->node_id))
        return CLOUD_STATUS_INDIRECT;

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

        char *claim_id = aclk_get_claimed_id();

        switch(status) {
            default:
            case CLOUD_STATUS_AVAILABLE:
                // the agent is not claimed
                buffer_json_member_add_string(wb, "url", cloud_url());
                buffer_json_member_add_string(wb, "reason", claim_agent_failure_reason_get());
                break;

            case CLOUD_STATUS_BANNED:
                // the agent is claimed, but has been banned from NC
                buffer_json_member_add_string(wb, "claim_id", claim_id);
                buffer_json_member_add_string(wb, "url", cloud_status_aclk_base_url());
                buffer_json_member_add_string(wb, "reason", "Agent is banned from Netdata Cloud");
                buffer_json_member_add_string(wb, "url", cloud_url());
                break;

            case CLOUD_STATUS_OFFLINE:
                // the agent is claimed, but cannot get online
                buffer_json_member_add_string(wb, "claim_id", claim_id);
                buffer_json_member_add_string(wb, "url", cloud_status_aclk_base_url());
                buffer_json_member_add_string(wb, "reason", cloud_status_aclk_offline_reason());
                if (next_connect > now_s) {
                    buffer_json_member_add_time_t(wb, "next_check", next_connect);
                    buffer_json_member_add_time_t(wb, "next_in", next_connect - now_s);
                }
                break;

            case CLOUD_STATUS_ONLINE:
                // the agent is claimed and online
                buffer_json_member_add_string(wb, "claim_id", claim_id);
                buffer_json_member_add_string(wb, "url", cloud_status_aclk_base_url());
                buffer_json_member_add_string(wb, "reason", "");
                break;

            case CLOUD_STATUS_INDIRECT:
                buffer_json_member_add_string(wb, "url", cloud_url());
                break;
        }

        freez(claim_id);
    }
    buffer_json_object_close(wb); // cloud

    return status;
}
