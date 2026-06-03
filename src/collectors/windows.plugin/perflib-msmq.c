// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"

struct msmq_queue {
    RRDSET *st_messages;
    RRDDIM *rd_messages;

    RRDSET *st_bytes;
    RRDDIM *rd_bytes;

    RRDSET *st_journal_messages;
    RRDDIM *rd_journal_messages;

    RRDSET *st_journal_bytes;
    RRDDIM *rd_journal_bytes;

    COUNTER_DATA messages_in_queue;
    COUNTER_DATA bytes_in_queue;
    COUNTER_DATA messages_in_journal_queue;
    COUNTER_DATA bytes_in_journal_queue;
};

static DICTIONARY *msmq_queues = NULL;

static void msmq_queue_insert_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    struct msmq_queue *mq = value;

    mq->messages_in_queue.key = "Messages in Queue";
    mq->bytes_in_queue.key = "Bytes in Queue";
    mq->messages_in_journal_queue.key = "Messages in Journal Queue";
    mq->bytes_in_journal_queue.key = "Bytes in Journal Queue";
}

static void initialize(void)
{
    msmq_queues = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct msmq_queue));

    dictionary_register_insert_callback(msmq_queues, msmq_queue_insert_cb, NULL);
}

static void msmq_queue_chart_id(char *dst, size_t size, const char *instance, const char *suffix)
{
    snprintfz(dst, size, "msmq_queue_%s_%s", instance, suffix);
    netdata_fix_chart_name(dst);
}

static void msmq_queue_chart_messages(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_messages)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(id, sizeof(id), queue, "messages");

        mq->st_messages = rrdset_create_localhost(
            "msmq_queue_messages",
            id,
            NULL,
            "queues",
            "msmq.queue_messages",
            "MSMQ messages in queue",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSMQ",
            PRIO_MSMQ_QUEUE_MESSAGES,
            update_every,
            RRDSET_TYPE_LINE);

        mq->rd_messages = rrddim_add(mq->st_messages, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrdlabels_add(mq->st_messages->rrdlabels, "queue", queue, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(mq->st_messages, mq->rd_messages, (collected_number)mq->messages_in_queue.current.Data);
    rrdset_done(mq->st_messages);
}

static void msmq_queue_chart_bytes(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_bytes)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(id, sizeof(id), queue, "bytes");

        mq->st_bytes = rrdset_create_localhost(
            "msmq_queue_bytes",
            id,
            NULL,
            "queues",
            "msmq.queue_bytes",
            "MSMQ bytes in queue",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSMQ",
            PRIO_MSMQ_QUEUE_BYTES,
            update_every,
            RRDSET_TYPE_LINE);

        mq->rd_bytes = rrddim_add(mq->st_bytes, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrdlabels_add(mq->st_bytes->rrdlabels, "queue", queue, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(mq->st_bytes, mq->rd_bytes, (collected_number)mq->bytes_in_queue.current.Data);
    rrdset_done(mq->st_bytes);
}

static void msmq_queue_chart_journal_messages(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_journal_messages)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(id, sizeof(id), queue, "journal_messages");

        mq->st_journal_messages = rrdset_create_localhost(
            "msmq_queue_journal_messages",
            id,
            NULL,
            "journal",
            "msmq.queue_journal_messages",
            "MSMQ messages in journal queue",
            "messages",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSMQ",
            PRIO_MSMQ_JOURNAL_MESSAGES,
            update_every,
            RRDSET_TYPE_LINE);

        mq->rd_journal_messages =
            rrddim_add(mq->st_journal_messages, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrdlabels_add(mq->st_journal_messages->rrdlabels, "queue", queue, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        mq->st_journal_messages, mq->rd_journal_messages, (collected_number)mq->messages_in_journal_queue.current.Data);
    rrdset_done(mq->st_journal_messages);
}

static void msmq_queue_chart_journal_bytes(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_journal_bytes)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(id, sizeof(id), queue, "journal_bytes");

        mq->st_journal_bytes = rrdset_create_localhost(
            "msmq_queue_journal_bytes",
            id,
            NULL,
            "journal",
            "msmq.queue_journal_bytes",
            "MSMQ bytes in journal queue",
            "bytes",
            PLUGIN_WINDOWS_NAME,
            "PerflibMSMQ",
            PRIO_MSMQ_JOURNAL_BYTES,
            update_every,
            RRDSET_TYPE_LINE);

        mq->rd_journal_bytes = rrddim_add(mq->st_journal_bytes, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrdlabels_add(mq->st_journal_bytes->rrdlabels, "queue", queue, RRDLABEL_SRC_AUTO);
    }

    rrddim_set_by_pointer(
        mq->st_journal_bytes, mq->rd_journal_bytes, (collected_number)mq->bytes_in_journal_queue.current.Data);
    rrdset_done(mq->st_journal_bytes);
}

static bool do_msmq(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "MSMQ Queue");
    if (!pObjectType)
        return false;

    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi)
            break;

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer)))
            strncpyz(windows_shared_buffer, "[unknown]", sizeof(windows_shared_buffer) - 1);

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        struct msmq_queue *mq = dictionary_set(msmq_queues, windows_shared_buffer, NULL, sizeof(*mq));

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->messages_in_queue))
            msmq_queue_chart_messages(mq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->bytes_in_queue))
            msmq_queue_chart_bytes(mq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->messages_in_journal_queue))
            msmq_queue_chart_journal_messages(mq, windows_shared_buffer, update_every);

        if (perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->bytes_in_journal_queue))
            msmq_queue_chart_journal_bytes(mq, windows_shared_buffer, update_every);
    }

    return true;
}

int do_PerflibMSMQ(int update_every, usec_t dt __maybe_unused)
{
    static bool initialized = false;

    if (unlikely(!initialized)) {
        initialize();
        initialized = true;
    }

    DWORD id = RegistryFindIDByName("MSMQ Queue");
    if (id == PERFLIB_REGISTRY_NAME_NOT_FOUND)
        return -1;

    PERF_DATA_BLOCK *pDataBlock = perflibGetPerformanceData(id);
    if (!pDataBlock)
        return -1;

    if (!do_msmq(pDataBlock, update_every))
        return -1;

    return 0;
}
