// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

/* Retrieve the claim id for the agent.
 * Caller owns the string.
*/
char *get_agent_claimid()
{
    char *result;
    rrdhost_aclk_state_lock(localhost);
    result = (localhost->aclk_state.claimed_id == NULL) ? NULL : strdupz(localhost->aclk_state.claimed_id);
    rrdhost_aclk_state_unlock(localhost);
    return result;
}

/* Change the claimed state of the agent.
 *
 * This only happens when the user has explicitly requested it:
 *   - via the cli tool by reloading the claiming state
 *   - after spawning the claim because of a command-line argument
 * If this happens with the ACLK active under an old claim then we MUST KILL THE LINK
 */
void load_claiming_state(void)
{
    nd_uuid_t uuid;

    // Propagate into aclk and registry. Be kind of atomic...
    cloud_url();

    rrdhost_aclk_state_lock(localhost);
    if (localhost->aclk_state.claimed_id) {
        if (aclk_connected)
            localhost->aclk_state.prev_claimed_id = strdupz(localhost->aclk_state.claimed_id);
        freez(localhost->aclk_state.claimed_id);
        localhost->aclk_state.claimed_id = NULL;
    }

    if (aclk_connected) {
        netdata_log_info("Agent was already connected to Cloud - forcing reconnection under new credentials");
        aclk_kill_link = 1;
    }
    aclk_disable_runtime = 0;

    char filename[FILENAME_MAX + 1];
    snprintfz(filename, FILENAME_MAX, "%s/claimed_id", netdata_configured_cloud_dir);

    long bytes_read;
    char *claimed_id = read_by_filename(filename, &bytes_read);
    if(!claimed_id) {
        const char *error;
        if(claim_agent_from_files(&error))
            claimed_id = read_by_filename(filename, &bytes_read);
        else
            nd_log(NDLS_DAEMON, NDLP_ERR, "Failed to automatically claim: %s", error);
    }

    if(claimed_id && uuid_parse(claimed_id, uuid)) {
        netdata_log_error("claimed_id \"%s\" doesn't look like valid UUID", claimed_id);
        freez(claimed_id);
        claimed_id = NULL;
    }

    if(claimed_id) {
        localhost->aclk_state.claimed_id = mallocz(UUID_STR_LEN);
        uuid_unparse_lower(uuid, localhost->aclk_state.claimed_id);
    }

    rrdhost_aclk_state_unlock(localhost);
    invalidate_node_instances(&localhost->host_uuid, claimed_id ? &uuid : NULL);
    metaqueue_store_claim_id(&localhost->host_uuid, claimed_id ? &uuid : NULL);

    if (!claimed_id) {
        netdata_log_info("Unable to load '%s', setting state to AGENT_UNCLAIMED", filename);
        return;
    }

    freez(claimed_id);

    netdata_log_info("File '%s' was found. Setting state to AGENT_CLAIMED.", filename);
}

void claim_reload_all(void) {
    nd_log_limits_unlimited();
    load_claiming_state();
    registry_update_cloud_base_url();
    rrdpush_send_claimed_id(localhost);
    nd_log_limits_reset();
}
