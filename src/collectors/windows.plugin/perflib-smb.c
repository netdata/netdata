// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct smb_share {
    usec_t last_collected;

    RRDSET *st_current_open_file_count;
    RRDSET *st_tree_connect_count;
    RRDSET *st_received_bytes;
    RRDSET *st_write_requests;
    RRDSET *st_read_requests;
    RRDSET *st_metadata_requests;
    RRDSET *st_sent_bytes;
    RRDSET *st_files_opened;

    RRDDIM *rd_current_open_file_count;
    RRDDIM *rd_tree_connect_count;
    RRDDIM *rd_received_bytes;
    RRDDIM *rd_write_requests;
    RRDDIM *rd_read_requests;
    RRDDIM *rd_metadata_requests;
    RRDDIM *rd_sent_bytes;
    RRDDIM *rd_files_opened;

    COUNTER_DATA currentOpenFileCount;
    COUNTER_DATA treeConnectCount;
    COUNTER_DATA receivedBytes;
    COUNTER_DATA writeRequests;
    COUNTER_DATA readRequests;
    COUNTER_DATA metadataRequests;
    COUNTER_DATA sentBytes;
    COUNTER_DATA filesOpened;
};

static DICTIONARY *smb_shares = NULL;

static void smb_share_initialize(struct smb_share *share)
{
    share->currentOpenFileCount.key = "Current Open File Count";
    share->treeConnectCount.key = "Tree Connect Count";
    share->receivedBytes.key = "Received Bytes/sec";
    share->writeRequests.key = "Write Requests/sec";
    share->readRequests.key = "Read Requests/sec";
    share->metadataRequests.key = "Metadata Requests/sec";
    share->sentBytes.key = "Sent Bytes/sec";
    share->filesOpened.key = "Files Opened/sec";
}

static void smb_share_cleanup(struct smb_share *share)
{
    rrdset_is_obsolete___safe_from_collector_thread(share->st_current_open_file_count);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_tree_connect_count);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_received_bytes);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_write_requests);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_read_requests);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_metadata_requests);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_sent_bytes);
    rrdset_is_obsolete___safe_from_collector_thread(share->st_files_opened);
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

static bool do_smb_server_shares(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "SMB Server Shares");
    if (!pObjectType)
        return false;

    usec_t now_ut = now_monotonic_usec();

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        struct smb_share *share = dictionary_set(smb_shares, windows_shared_buffer, NULL, sizeof(*share));
        share->last_collected = now_ut;

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->currentOpenFileCount);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->treeConnectCount);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->receivedBytes);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->writeRequests);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->readRequests);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->metadataRequests);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->sentBytes);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &share->filesOpened);

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
            &share->st_tree_connect_count,
            &share->rd_tree_connect_count,
            &share->treeConnectCount,
            update_every,
            windows_shared_buffer,
            "tree_connect_count",
            "smb.server_shares_tree_connect_count",
            "Tree connect count on the SMB share",
            "connections",
            PRIO_SMB_SERVER_SHARES_TREE_CONNECT_COUNT,
            "connections");

        smb_share_chart(
            &share->st_received_bytes,
            &share->rd_received_bytes,
            &share->receivedBytes,
            update_every,
            windows_shared_buffer,
            "received_bytes",
            "smb.server_shares_received_bytes",
            "Received bytes on the SMB share",
            "bytes/s",
            PRIO_SMB_SERVER_SHARES_RECEIVED_BYTES,
            "received");

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
            &share->st_sent_bytes,
            &share->rd_sent_bytes,
            &share->sentBytes,
            update_every,
            windows_shared_buffer,
            "sent_bytes",
            "smb.server_shares_sent_bytes",
            "Sent bytes on the SMB share",
            "bytes/s",
            PRIO_SMB_SERVER_SHARES_SENT_BYTES,
            "sent");

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
    }

    // delete entries not seen in this collection cycle
    {
        struct smb_share *share;
        dfe_start_write(smb_shares, share)
        {
            if (share->last_collected < now_ut)
                dictionary_del(smb_shares, share_dfe.name);
        }
        dfe_done(share);
        dictionary_garbage_collect(smb_shares);
    }

    return true;
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

    DWORD id = RegistryFindIDByName("SMB Server Shares");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    do_smb_server_shares(pDataBlock, update_every);

    return 0;
}
