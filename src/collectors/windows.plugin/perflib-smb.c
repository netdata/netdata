// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

// The registry view of "SMB Server Shares" attaches values to the wrong counters on some Windows builds
// (reads, metadata and files opened; issue #24660), so this module reads it through PDH.
#define SMB_SERVER_SHARES_OBJECT "SMB Server Shares"

// While the object is registered but PDH cannot add its counters (e.g. the server service is stopped).
#define SMB_QUERY_RETRY_SECONDS 60

// Consecutive failed samples after which the query is rebuilt: its handles may be stale after the provider
// was re-registered, and an unchanged query would never recover.
#define SMB_QUERY_MAX_FAILED_SAMPLES 60

enum smb_share_counter {
    SMB_SHARE_CURRENT_OPEN_FILE_COUNT = 0,
    SMB_SHARE_WRITE_REQUESTS,
    SMB_SHARE_READ_REQUESTS,
    SMB_SHARE_METADATA_REQUESTS,
    SMB_SHARE_FILES_OPENED,
    SMB_SHARE_TREE_CONNECT_COUNT,
    SMB_SHARE_RECEIVED_BYTES,
    SMB_SHARE_SENT_BYTES,

    // terminator
    SMB_SHARE_COUNTERS_MAX,
};

static const char *const smb_share_counter_names[SMB_SHARE_COUNTERS_MAX] = {
    [SMB_SHARE_CURRENT_OPEN_FILE_COUNT] = "Current Open File Count",
    [SMB_SHARE_WRITE_REQUESTS] = "Write Requests/sec",
    [SMB_SHARE_READ_REQUESTS] = "Read Requests/sec",
    [SMB_SHARE_METADATA_REQUESTS] = "Metadata Requests/sec",
    [SMB_SHARE_FILES_OPENED] = "Files Opened/sec",
    [SMB_SHARE_TREE_CONNECT_COUNT] = "Tree Connect Count",
    [SMB_SHARE_RECEIVED_BYTES] = "Received Bytes/sec",
    [SMB_SHARE_SENT_BYTES] = "Sent Bytes/sec",
};

struct smb_share {
    usec_t last_collected;

    RRDSET *st_current_open_file_count;
    RRDSET *st_write_requests;
    RRDSET *st_read_requests;
    RRDSET *st_metadata_requests;
    RRDSET *st_files_opened;
    RRDSET *st_tree_connects;
    RRDSET *st_traffic;

    RRDDIM *rd_current_open_file_count;
    RRDDIM *rd_write_requests;
    RRDDIM *rd_read_requests;
    RRDDIM *rd_metadata_requests;
    RRDDIM *rd_files_opened;
    RRDDIM *rd_tree_connects;
    RRDDIM *rd_received_bytes;
    RRDDIM *rd_sent_bytes;

    COUNTER_DATA counters[SMB_SHARE_COUNTERS_MAX];
};

static DICTIONARY *smb_shares = NULL;
static ND_PDH_QUERY *smb_query = NULL;
static time_t smb_query_next_attempt = 0;
static size_t smb_query_failed_samples = 0;
static bool smb_query_partial = false; // some counters could not be added

static void smb_share_initialize(struct smb_share *share)
{
    for (size_t c = 0; c < SMB_SHARE_COUNTERS_MAX; c++)
        share->counters[c].key = smb_share_counter_names[c];
}

static void smb_share_cleanup(struct smb_share *share)
{
    rrdset_is_obsolete___safe_from_collector_thread(share->st_current_open_file_count);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_write_requests);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_read_requests);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_metadata_requests);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_files_opened);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_tree_connects);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_traffic);
}

static void dict_smb_share_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct smb_share *share = value;

    smb_share_initialize(share);
}

static void dict_smb_share_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct smb_share *share = value;

    smb_share_cleanup(share);
}

static void initialize(void)
{
    smb_shares = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct smb_share));

    dictionary_register_insert_callback(smb_shares, dict_smb_share_insert_cb, NULL);
    dictionary_register_delete_callback(smb_shares, dict_smb_share_delete_cb, NULL);
}

static void smb_share_chart(
    RRDSET **st,
    RRDDIM **rd,
    COUNTER_DATA *counter,
    int update_every,
    const char *share,
    const char *id_suffix,
    const char *context,
    const char *title,
    const char *units,
    int priority,
    const char *dimension)
{
    if (!counter->updated)
        return;

    if (unlikely(!*st)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, sizeof(id), "smb_share_%s_%s", share, id_suffix);
        netdata_fix_chart_name(id);

        *st = rrdset_create_localhost(
            "smb",
            id,
            NULL,
            "shares",
            context,
            title,
            units,
            PLUGIN_WINDOWS_NAME,
            "PerflibSMB",
            priority,
            update_every,
            RRDSET_TYPE_LINE);

        *rd = perflib_rrddim_add(*st, dimension, NULL, 1, 1, counter);
        rrdlabels_add((*st)->rrdlabels, "share", share, RRDLABEL_SRC_AUTO);
    }

    perflib_rrddim_set_by_pointer(*st, *rd, counter);
    rrdset_done(*st);
}

static void smb_share_traffic_chart(struct smb_share *share, int update_every, const char *name)
{
    COUNTER_DATA *received = &share->counters[SMB_SHARE_RECEIVED_BYTES];
    COUNTER_DATA *sent = &share->counters[SMB_SHARE_SENT_BYTES];

    if (!received->updated && !sent->updated)
        return;

    if (unlikely(!share->st_traffic)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        snprintfz(id, sizeof(id), "smb_share_%s_traffic", name);
        netdata_fix_chart_name(id);

        share->st_traffic = rrdset_create_localhost(
            "smb",
            id,
            NULL,
            "shares",
            "smb.server_shares_traffic",
            "Traffic on the SMB share",
            "bytes/s",
            PLUGIN_WINDOWS_NAME,
            "PerflibSMB",
            PRIO_SMB_SERVER_SHARES_TRAFFIC,
            update_every,
            RRDSET_TYPE_AREA);

        rrdlabels_add(share->st_traffic->rrdlabels, "share", name, RRDLABEL_SRC_AUTO);
    }

    // Each dimension is added on its first successful read so its algorithm follows the real counter type.
    if (received->updated) {
        if (unlikely(!share->rd_received_bytes))
            share->rd_received_bytes = perflib_rrddim_add(share->st_traffic, "received", NULL, 1, 1, received);
        perflib_rrddim_set_by_pointer(share->st_traffic, share->rd_received_bytes, received);
    }

    if (sent->updated) {
        if (unlikely(!share->rd_sent_bytes))
            share->rd_sent_bytes = perflib_rrddim_add(share->st_traffic, "sent", NULL, -1, 1, sent);
        perflib_rrddim_set_by_pointer(share->st_traffic, share->rd_sent_bytes, sent);
    }

    rrdset_done(share->st_traffic);
}

static void smb_share_charts(struct smb_share *share, int update_every, const char *name)
{
    smb_share_chart(
        &share->st_current_open_file_count,
        &share->rd_current_open_file_count,
        &share->counters[SMB_SHARE_CURRENT_OPEN_FILE_COUNT],
        update_every,
        name,
        "current_open_file_count",
        "smb.server_shares_current_open_file_count",
        "Current open file count on the SMB share",
        "files",
        PRIO_SMB_SERVER_SHARES_CURRENT_OPEN_FILE_COUNT,
        "open");

    smb_share_chart(
        &share->st_write_requests,
        &share->rd_write_requests,
        &share->counters[SMB_SHARE_WRITE_REQUESTS],
        update_every,
        name,
        "write_requests",
        "smb.server_shares_write_requests",
        "Write requests on the SMB share",
        "requests/s",
        PRIO_SMB_SERVER_SHARES_WRITE_REQUESTS,
        "writes");

    smb_share_chart(
        &share->st_read_requests,
        &share->rd_read_requests,
        &share->counters[SMB_SHARE_READ_REQUESTS],
        update_every,
        name,
        "read_requests",
        "smb.server_shares_read_requests",
        "Read requests on the SMB share",
        "requests/s",
        PRIO_SMB_SERVER_SHARES_READ_REQUESTS,
        "reads");

    smb_share_chart(
        &share->st_metadata_requests,
        &share->rd_metadata_requests,
        &share->counters[SMB_SHARE_METADATA_REQUESTS],
        update_every,
        name,
        "metadata_requests",
        "smb.server_shares_metadata_requests",
        "Metadata requests on the SMB share",
        "requests/s",
        PRIO_SMB_SERVER_SHARES_METADATA_REQUESTS,
        "metadata");

    smb_share_chart(
        &share->st_files_opened,
        &share->rd_files_opened,
        &share->counters[SMB_SHARE_FILES_OPENED],
        update_every,
        name,
        "files_opened",
        "smb.server_shares_files_opened",
        "Files opened on the SMB share",
        "files/s",
        PRIO_SMB_SERVER_SHARES_FILES_OPENED,
        "opened");

    smb_share_chart(
        &share->st_tree_connects,
        &share->rd_tree_connects,
        &share->counters[SMB_SHARE_TREE_CONNECT_COUNT],
        update_every,
        name,
        "tree_connects",
        "smb.server_shares_tree_connects",
        "Tree connects on the SMB share",
        "connections",
        PRIO_SMB_SERVER_SHARES_TREE_CONNECTS,
        "connects");

    smb_share_traffic_chart(share, update_every, name);
}

static void smb_shares_evict_all(void)
{
    struct smb_share *share;
    dfe_start_write(smb_shares, share)
    {
        dictionary_del(smb_shares, share_dfe.name);
    }
    dfe_done(share);
    dictionary_garbage_collect(smb_shares);
}

struct smb_sample {
    usec_t now_ut;
    enum smb_share_counter counter;
    bool complete;
};

static void smb_share_instance_cb(const char *instance, const RAW_DATA *raw, void *data)
{
    struct smb_sample *sample = data;

    if (!instance || !*instance) {
        // Distinct shares cannot share one identity; skip it and keep the others from being evicted.
        sample->complete = false;
        return;
    }

    // The aggregate would double-count when shares are summed.
    if (strcasecmp(instance, "_Total") == 0)
        return;

    struct smb_share *share = dictionary_set(smb_shares, instance, NULL, sizeof(*share));
    if (share->last_collected != sample->now_ut) {
        // First counter to report this share in this sample: values from the previous sample are not current.
        share->last_collected = sample->now_ut;
        for (size_t c = 0; c < SMB_SHARE_COUNTERS_MAX; c++)
            share->counters[c].updated = false;
    }

    // The share exists, but this counter has no valid value for it now.
    if (!raw)
        return;

    COUNTER_DATA *cd = &share->counters[sample->counter];
    cd->previous = cd->current;
    cd->current = *raw;
    cd->updated = true;
}

// Returns false when the sample or a counter read failed; charts are kept and show a gap.
static bool do_smb_server_shares(int update_every)
{
    if (!nd_pdh_query_collect(smb_query))
        return false;

    struct smb_sample sample = {
        .now_ut = now_monotonic_usec(),
        .complete = true,
    };

    bool have_instances = false;
    bool read_failed = false;
    for (size_t c = 0; c < SMB_SHARE_COUNTERS_MAX; c++) {
        if (!nd_pdh_query_has_counter(smb_query, c))
            continue;

        sample.counter = (enum smb_share_counter)c;
        switch (nd_pdh_query_read(smb_query, c, smb_share_instance_cb, &sample)) {
            case ND_PDH_READ_OK:
                have_instances = true;
                break;

            case ND_PDH_READ_NO_DATA:
                break;

            case ND_PDH_READ_FAILED:
            default:
                sample.complete = false;
                read_failed = true;
                break;
        }
    }

    if (!have_instances) {
        if (!sample.complete)
            return false;

        // The sample succeeded without any instance, so no shares remain.
        smb_shares_evict_all();
        return true;
    }

    struct smb_share *share;
    dfe_start_write(smb_shares, share)
    {
        if (share->last_collected == sample.now_ut)
            smb_share_charts(share, update_every, share_dfe.name);
        else if (sample.complete)
            // Only a complete sample proves that a share disappeared.
            dictionary_del(smb_shares, share_dfe.name);
    }
    dfe_done(share);
    dictionary_garbage_collect(smb_shares);

    // A counter that cannot be read while others can may also have a stale handle.
    return !read_failed;
}

static void smb_query_close(void)
{
    nd_pdh_query_destroy(smb_query);
    smb_query = NULL;
    smb_query_failed_samples = 0;
    smb_query_partial = false;
}

static bool smb_query_has_all_counters(void)
{
    for (size_t c = 0; c < SMB_SHARE_COUNTERS_MAX; c++) {
        if (!nd_pdh_query_has_counter(smb_query, c))
            return false;
    }
    return true;
}

void do_PerflibSMB_cleanup(void)
{
    smb_query_close();

    dictionary_destroy(smb_shares);
    smb_shares = NULL;
}

int do_PerflibSMB(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    // Keep probing while the server role is absent so a later install is detected without a restart.
    // The name lookup is in memory; PDH is opened only once the object is registered.
    if (RegistryFindIDByName(SMB_SERVER_SHARES_OBJECT) == PERFLIB_REGISTRY_NAME_NOT_FOUND) {
        smb_query_close();
        smb_shares_evict_all();
        return 0;
    }

    if (unlikely(!smb_query)) {
        time_t now = now_monotonic_sec();
        if (now < smb_query_next_attempt)
            return 0;

        smb_query = nd_pdh_query_create(SMB_SERVER_SHARES_OBJECT, smb_share_counter_names, SMB_SHARE_COUNTERS_MAX);
        if (!smb_query) {
            smb_query_next_attempt = now + SMB_QUERY_RETRY_SECONDS;
            return 0;
        }

        // Only a partial query waits before its rebuild; a complete one can be recreated at once after a close.
        smb_query_partial = !smb_query_has_all_counters();
        if (smb_query_partial)
            smb_query_next_attempt = now + SMB_QUERY_RETRY_SECONDS;
    }

    // A rebuilt query is created on the next iteration; existing charts stay until a complete sample proves them gone.
    if (do_smb_server_shares(update_every))
        smb_query_failed_samples = 0;
    else if (++smb_query_failed_samples >= SMB_QUERY_MAX_FAILED_SAMPLES) {
        smb_query_close();
        return 0;
    }

    // A counter that failed to be added is otherwise never read again; rebuilding retries it.
    if (smb_query_partial && now_monotonic_sec() >= smb_query_next_attempt)
        smb_query_close();

    return 0;
}
