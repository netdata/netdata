// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

// --------------------------------------------------------------------------------------------------------------------
// keep track of the last claiming failure reason

static char cloud_claim_failure_reason[4096] = "";

void claim_agent_failure_reason_set(const char *format, ...) {
    if(!format || !*format) {
        cloud_claim_failure_reason[0] = '\0';
        return;
    }

    va_list args;
    va_start(args, format);
    vsnprintf(cloud_claim_failure_reason, sizeof(cloud_claim_failure_reason), format, args);
    va_end(args);

    nd_log(NDLS_DAEMON, NDLP_ERR,
           "CLAIM: %s", cloud_claim_failure_reason);
}

const char *claim_agent_failure_reason_get(void) {
    if(!cloud_claim_failure_reason[0])
        return "Agent is not claimed yet";
    else
        return cloud_claim_failure_reason;
}

// --------------------------------------------------------------------------------------------------------------------
// claimed_id load/save

bool claimed_id_save_to_file(const char *claimed_id_str) {
    bool ret;
    const char *filename = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "claimed_id");
    FILE *fp = fopen(filename, "w");
    if(fp) {
        fprintf(fp, "%s", claimed_id_str);
        fclose(fp);
        ret = true;
    }
    else {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CLAIM: cannot open file '%s' for writing.", filename);
        ret = false;
    }

    freez((void *)filename);
    return ret;
}

static ND_UUID claimed_id_parse(const char *claimed_id, const char *source) {
    ND_UUID uuid;

    if(uuid_parse_flexi(claimed_id, uuid.uuid) != 0) {
        uuid = UUID_ZERO;
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CLAIM: claimed_id '%s' (loaded from '%s'), is not a valid UUID.",
               claimed_id, source);
    }

    return uuid;
}

static ND_UUID claimed_id_load_from_file(void) {
    ND_UUID uuid;

    long bytes_read;
    const char *filename = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "claimed_id");
    char *claimed_id = read_by_filename(filename, &bytes_read);

    if(!claimed_id)
        uuid = UUID_ZERO;
    else
        uuid = claimed_id_parse(claimed_id, filename);

    freez(claimed_id);
    freez((void *)filename);
    return uuid;
}

static ND_UUID claimed_id_get_from_cloud_conf(void) {
    if(inicfg_exists(&cloud_config, CONFIG_SECTION_GLOBAL, "claimed_id")) {
        const char *claimed_id = inicfg_get(&cloud_config, CONFIG_SECTION_GLOBAL, "claimed_id", "");
        if(claimed_id && *claimed_id)
            return claimed_id_parse(claimed_id, "cloud.conf");
    }
    return UUID_ZERO;
}

static ND_UUID claimed_id_load(void) {
    ND_UUID uuid = claimed_id_get_from_cloud_conf();
    if(UUIDiszero(uuid))
        uuid = claimed_id_load_from_file();

    return uuid;
}

bool is_agent_claimed(void) {
    ND_UUID uuid = claim_id_get_uuid();
    return !UUIDiszero(uuid);
}

// --------------------------------------------------------------------------------------------------------------------

bool claim_id_matches(const char *claim_id) {
    ND_UUID this_one = UUID_ZERO;
    if(uuid_parse_flexi(claim_id, this_one.uuid) != 0 || UUIDiszero(this_one))
        return false;

    ND_UUID having = claim_id_get_uuid();
    if(!UUIDiszero(having) && UUIDeq(having, this_one))
        return true;

    return false;
}

bool claim_id_matches_any(const char *claim_id) {
    ND_UUID this_one = UUID_ZERO;
    if(uuid_parse_flexi(claim_id, this_one.uuid) != 0 || UUIDiszero(this_one))
        return false;

    ND_UUID having = claim_id_get_uuid();
    if(!UUIDiszero(having) && UUIDeq(having, this_one))
        return true;

    having = localhost->aclk.claim_id_of_parent;
    if(!UUIDiszero(having) && UUIDeq(having, this_one))
        return true;

    having = localhost->aclk.claim_id_of_origin;
    if(!UUIDiszero(having) && UUIDeq(having, this_one))
        return true;

    return false;
}

/* Change the claimed state of the agent.
 *
 * This only happens when the user has explicitly requested it:
 *   - via the cli tool by reloading the claiming state
 *   - after spawning the claim because of a command-line argument
 * If this happens with the ACLK active under an old claim then we MUST KILL THE LINK
 */
bool load_claiming_state(void) {
    if (aclk_online()) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CLAIM: agent was already connected to NC - forcing reconnection under new credentials");
        disconnect_req = ACLK_RELOAD_CONF;
    }
    aclk_disable_runtime = 0;

    ND_UUID uuid = claimed_id_load();
    if(UUIDiszero(uuid)) {
        // not found
        if(claim_agent_automatically())
            uuid = claimed_id_load();
    }

    bool have_claimed_id = false;
    if(!UUIDiszero(uuid)) {
        // we go it somehow
        claim_id_set(uuid);
        have_claimed_id = true;
    }

    invalidate_node_instances(&localhost->host_id.uuid, have_claimed_id ? &uuid.uuid : NULL);
    metaqueue_store_claim_id(&localhost->host_id.uuid, have_claimed_id ? &uuid.uuid : NULL);

    errno_clear();

    if (!have_claimed_id)
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CLAIM: Unable to find our claimed_id, setting state to AGENT_UNCLAIMED");
    else
        nd_log(NDLS_DAEMON, NDLP_INFO,
               "CLAIM: Found a valid claimed_id, setting state to AGENT_CLAIMED");

    return have_claimed_id;
}

CLOUD_STATUS claim_reload_and_wait_online(void) {
    nd_log(NDLS_DAEMON, NDLP_INFO,
           "CLAIM: Reloading Agent Claiming configuration.");

    nd_log_limits_unlimited();
    cloud_conf_load(0);
    bool claimed = load_claiming_state();
    registry_update_cloud_base_url();
    stream_sender_send_claimed_id(localhost);
    nd_log_limits_reset();

    CLOUD_STATUS status = cloud_status();
    if(claimed) {
        int ms = 0;
        do {
            status = cloud_status();
            if ((status == CLOUD_STATUS_ONLINE) && !UUIDiszero(localhost->node_id))
                break;

            sleep_usec(50 * USEC_PER_MS);
            ms += 50;
        } while (ms < 10000);
    }

    return status;
}
