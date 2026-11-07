// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

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

    COUNTER_DATA currentOpenFileCount;
    COUNTER_DATA writeRequests;
    COUNTER_DATA readRequests;
    COUNTER_DATA metadataRequests;
    COUNTER_DATA filesOpened;
    COUNTER_DATA treeConnectCount;
    COUNTER_DATA receivedBytes;
    COUNTER_DATA sentBytes;
};

static DICTIONARY *smb_shares = NULL;

static void smb_share_initialize(struct smb_share *share)
{
    share->currentOpenFileCount.key = "Current Open File Count";
    share->writeRequests.key = "Write Requests/sec";
    share->readRequests.key = "Read Requests/sec";
    share->metadataRequests.key = "Metadata Requests/sec";
    share->filesOpened.key = "Files Opened/sec";
    share->treeConnectCount.key = "Tree Connect Count";
    share->receivedBytes.key = "Received Bytes/sec";
    share->sentBytes.key = "Sent Bytes/sec";
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
    if (!share->receivedBytes.updated && !share->sentBytes.updated)
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
    if (share->receivedBytes.updated) {
        if (unlikely(!share->rd_received_bytes))
            share->rd_received_bytes = perflib_rrddim_add(share->st_traffic, "received", NULL, 1, 1, &share->receivedBytes);
        perflib_rrddim_set_by_pointer(share->st_traffic, share->rd_received_bytes, &share->receivedBytes);
    }

    if (share->sentBytes.updated) {
        if (unlikely(!share->rd_sent_bytes))
            share->rd_sent_bytes = perflib_rrddim_add(share->st_traffic, "sent", NULL, -1, 1, &share->sentBytes);
        perflib_rrddim_set_by_pointer(share->st_traffic, share->rd_sent_bytes, &share->sentBytes);
    }

    rrdset_done(share->st_traffic);
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

static void do_smb_server_shares(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "SMB Server Shares");
    if (!pObjectType) {
        // The query succeeded without the object, so no shares remain in this snapshot.
        smb_shares_evict_all();
        return;
    }

    usec_t now_ut = now_monotonic_usec();
    bool complete = true;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            // Distinct shares cannot share one identity; skip it and keep the others from being evicted.
            complete = false;
            continue;
        }

        // The aggregate would double-count when shares are summed.
        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct smb_share *share = dictionary_set(smb_shares, windows_shared_buffer, NULL, sizeof(*share));
        share->last_collected = now_ut;

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->currentOpenFileCount);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->writeRequests);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->readRequests);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->metadataRequests);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->filesOpened);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->treeConnectCount);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->receivedBytes);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->sentBytes);

        smb_share_chart(
            &share->st_current_open_file_count,
            &share->rd_current_open_file_count,
            &share->currentOpenFileCount,
            update_every,
            windows_shared_buffer,
            "current_open_file_count",
            "smb.server_shares_current_open_file_count",
            "Current open file count on the SMB share",
            "files",
            PRIO_SMB_SERVER_SHARES_CURRENT_OPEN_FILE_COUNT,
            "open");

        smb_share_chart(
            &share->st_write_requests,
            &share->rd_write_requests,
            &share->writeRequests,
            update_every,
            windows_shared_buffer,
            "write_requests",
            "smb.server_shares_write_requests",
            "Write requests on the SMB share",
            "requests/s",
            PRIO_SMB_SERVER_SHARES_WRITE_REQUESTS,
            "writes");

        smb_share_chart(
            &share->st_read_requests,
            &share->rd_read_requests,
            &share->readRequests,
            update_every,
            windows_shared_buffer,
            "read_requests",
            "smb.server_shares_read_requests",
            "Read requests on the SMB share",
            "requests/s",
            PRIO_SMB_SERVER_SHARES_READ_REQUESTS,
            "reads");

        smb_share_chart(
            &share->st_metadata_requests,
            &share->rd_metadata_requests,
            &share->metadataRequests,
            update_every,
            windows_shared_buffer,
            "metadata_requests",
            "smb.server_shares_metadata_requests",
            "Metadata requests on the SMB share",
            "requests/s",
            PRIO_SMB_SERVER_SHARES_METADATA_REQUESTS,
            "metadata");

        smb_share_chart(
            &share->st_files_opened,
            &share->rd_files_opened,
            &share->filesOpened,
            update_every,
            windows_shared_buffer,
            "files_opened",
            "smb.server_shares_files_opened",
            "Files opened on the SMB share",
            "files/s",
            PRIO_SMB_SERVER_SHARES_FILES_OPENED,
            "opened");

        smb_share_chart(
            &share->st_tree_connects,
            &share->rd_tree_connects,
            &share->treeConnectCount,
            update_every,
            windows_shared_buffer,
            "tree_connects",
            "smb.server_shares_tree_connects",
            "Tree connects on the SMB share",
            "connections",
            PRIO_SMB_SERVER_SHARES_TREE_CONNECTS,
            "connects");

        smb_share_traffic_chart(share, update_every, windows_shared_buffer);
    }

    // An incomplete snapshot cannot prove which shares disappeared.
    if (complete) {
        struct smb_share *share;
        dfe_start_write(smb_shares, share)
        {
            if (share->last_collected < now_ut)
                dictionary_del(smb_shares, share_dfe.name);
        }
        dfe_done(share);
        dictionary_garbage_collect(smb_shares);
    }
}

void do_PerflibSMB_cleanup(void)
{
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
    DWORD id = RegistryFindIDByName("SMB Server Shares");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND) {
        smb_shares_evict_all();
        return 0;
    }

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        // A failed Perflib query is transient; keep existing charts and show a gap.
        return 0;

    do_smb_server_shares(pDataBlock, update_every);

    return 0;
}
