// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows_plugin.h"
#include "windows-internals.h"
#include "libnetdata/sanitizers/chart_id_and_name.h"

#include <ctype.h>

#define CONFIG_SECTION_PERFLIB_MSMQ "plugin:windows:PerflibMSMQ"
#define MSMQ_DEFAULT_INDIVIDUAL_QUEUES 100

struct msmq_queue;

typedef struct {
    DICT_ITEM_CONST DICTIONARY_ITEM *item;
    struct msmq_queue *mq;
} MSMQ_SAMPLE;

struct msmq_queue {
    usec_t last_collected;
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

static int msmq_sample_compare(const void *a, const void *b)
{
    const MSMQ_SAMPLE *sa = a, *sb = b;
    const char *name_a = dictionary_acquired_item_name(sa->item);
    const char *name_b = dictionary_acquired_item_name(sb->item);
    int cmp = strcasecmp(name_a, name_b);
    return cmp ? cmp : strcmp(name_a, name_b);
}

static char *msmq_normalize_patterns(const char *input)
{
    char *source = strdupz(input);
    char *patterns = mallocz((strlen(source) * 2) + 1);
    char *read = source, *write = patterns;
    bool first = true;

    while (*read) {
        char *start = read;
        while (*read && *read != ',' && *read != '|') {
            if (*read == '\\' && read[1])
                read += 2;
            else
                read++;
        }

        char *end = read;
        while (start < end && isspace((unsigned char)*start))
            start++;
        while (end > start && isspace((unsigned char)end[-1]))
            end--;

        if (start < end) {
            if (!first)
                *write++ = ',';
            bool token_start = true;
            while (start < end) {
                if (*start == '\\' && start + 1 < end) {
                    char next = start[1];
                    if (next == ',' || next == '|' || (next == '!' && token_start)) {
                        *write++ = *start++;
                        *write++ = *start++;
                        token_start = false;
                    } else {
                        *write++ = '\\';
                        *write++ = '\\';
                        start++;
                        token_start = false;
                    }
                } else if (*start == '\\') {
                    *write++ = '\\';
                    *write++ = '\\';
                    start++;
                    token_start = false;
                } else {
                    *write++ = *start++;
                    token_start = false;
                }
            }
            first = false;
        }
        if (*read)
            read++;
    }

    *write = '\0';
    freez(source);
    return patterns;
}

static DICTIONARY *msmq_queues = NULL;
static SIMPLE_PATTERN *queues_to_chart = NULL;
static long long max_individual_queues = MSMQ_DEFAULT_INDIVIDUAL_QUEUES;
static RRDSET *other_st_messages, *other_st_bytes, *other_st_journal_messages, *other_st_journal_bytes;
static RRDDIM *other_rd_messages, *other_rd_bytes, *other_rd_journal_messages, *other_rd_journal_bytes;

static void msmq_queue_obsolete_charts(struct msmq_queue *mq)
{
    rrdset_is_obsolete___safe_from_collector_thread(mq->st_messages);
    rrdset_is_obsolete___safe_from_collector_thread(mq->st_bytes);
    rrdset_is_obsolete___safe_from_collector_thread(mq->st_journal_messages);
    rrdset_is_obsolete___safe_from_collector_thread(mq->st_journal_bytes);
}

static void msmq_queue_delete_cb(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused)
{
    msmq_queue_obsolete_charts(value);
}

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
    char *patterns = msmq_normalize_patterns(
        inicfg_get(&netdata_config, CONFIG_SECTION_PERFLIB_MSMQ, "queues to chart individually", "*"));
    queues_to_chart = simple_pattern_create(patterns, ",|", SIMPLE_PATTERN_EXACT, false);
    freez(patterns);
    max_individual_queues = inicfg_get_number(
        &netdata_config, CONFIG_SECTION_PERFLIB_MSMQ, "max individually charted queues", MSMQ_DEFAULT_INDIVIDUAL_QUEUES);
    if (max_individual_queues < 0)
        max_individual_queues = MSMQ_DEFAULT_INDIVIDUAL_QUEUES;

    msmq_queues = dictionary_create_advanced(
        DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE, NULL, sizeof(struct msmq_queue));

    dictionary_register_insert_callback(msmq_queues, msmq_queue_insert_cb, NULL);
    dictionary_register_delete_callback(msmq_queues, msmq_queue_delete_cb, NULL);
}

static void msmq_queue_chart_id(
    char *dst,
    size_t size,
    const char *chart_type,
    size_t chart_type_length,
    const char *instance,
    const char *suffix,
    size_t suffix_length)
{
    const size_t instance_length = strnlen(instance, sizeof(windows_shared_buffer));
    XXH64_hash_t hash = XXH3_64bits(instance, instance_length);
    const size_t fixed_length = sizeof("msmq_queue_") - 1 + 1 + 16 + 1 + suffix_length;
    const size_t full_id_limit = RRD_ID_LENGTH_MAX - 1;
    const size_t type_prefix_length = chart_type_length + 1;
    size_t chart_id_limit = full_id_limit > type_prefix_length ? full_id_limit - type_prefix_length : 0;
    if (size && chart_id_limit >= size)
        chart_id_limit = size - 1;
    size_t visible_instance_length = chart_id_limit > fixed_length ? chart_id_limit - fixed_length : 0;
    snprintfz(
        dst,
        size,
        "msmq_queue_%.*s_%016llx_%s",
        (int)visible_instance_length,
        instance,
        (unsigned long long)hash,
        suffix);
    netdata_fix_chart_name(dst);
}

static void msmq_queue_chart_messages(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_messages)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(
            id,
            sizeof(id),
            "msmq_queue_messages",
            sizeof("msmq_queue_messages") - 1,
            queue,
            "messages",
            sizeof("messages") - 1);

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

    rrdset_isnot_obsolete___safe_from_collector_thread(mq->st_messages);
    rrddim_set_by_pointer(mq->st_messages, mq->rd_messages, (collected_number)mq->messages_in_queue.current.Data);
    rrdset_done(mq->st_messages);
}

static void msmq_queue_chart_bytes(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_bytes)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(
            id,
            sizeof(id),
            "msmq_queue_bytes",
            sizeof("msmq_queue_bytes") - 1,
            queue,
            "bytes",
            sizeof("bytes") - 1);

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

    rrdset_isnot_obsolete___safe_from_collector_thread(mq->st_bytes);
    rrddim_set_by_pointer(mq->st_bytes, mq->rd_bytes, (collected_number)mq->bytes_in_queue.current.Data);
    rrdset_done(mq->st_bytes);
}

static void msmq_queue_chart_journal_messages(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_journal_messages)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(
            id,
            sizeof(id),
            "msmq_queue_journal_messages",
            sizeof("msmq_queue_journal_messages") - 1,
            queue,
            "journal_messages",
            sizeof("journal_messages") - 1);

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

    rrdset_isnot_obsolete___safe_from_collector_thread(mq->st_journal_messages);
    rrddim_set_by_pointer(
        mq->st_journal_messages, mq->rd_journal_messages, (collected_number)mq->messages_in_journal_queue.current.Data);
    rrdset_done(mq->st_journal_messages);
}

static void msmq_queue_chart_journal_bytes(struct msmq_queue *mq, const char *queue, int update_every)
{
    if (unlikely(!mq->st_journal_bytes)) {
        char id[RRD_ID_LENGTH_MAX + 1];
        msmq_queue_chart_id(
            id,
            sizeof(id),
            "msmq_queue_journal_bytes",
            sizeof("msmq_queue_journal_bytes") - 1,
            queue,
            "journal_bytes",
            sizeof("journal_bytes") - 1);

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

    rrdset_isnot_obsolete___safe_from_collector_thread(mq->st_journal_bytes);
    rrddim_set_by_pointer(
        mq->st_journal_bytes, mq->rd_journal_bytes, (collected_number)mq->bytes_in_journal_queue.current.Data);
    rrdset_done(mq->st_journal_bytes);
}

static void msmq_chart_other(
    RRDSET **st,
    RRDDIM **rd,
    const char *chart_type,
    const char *id,
    const char *family,
    const char *context,
    const char *title,
    const char *units,
    int priority,
    uint64_t value,
    int update_every)
{
    if (unlikely(!*st)) {
        *st = rrdset_create_localhost(
            chart_type,
            id,
            NULL,
            family,
            context,
            title,
            units,
            PLUGIN_WINDOWS_NAME,
            "PerflibMSMQ",
            priority,
            update_every,
            RRDSET_TYPE_LINE);

        *rd = rrddim_add(*st, "queued", NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE);
        rrdlabels_add((*st)->rrdlabels, "queue", "Other (aggregated)", RRDLABEL_SRC_AUTO);
    }

    rrdset_isnot_obsolete___safe_from_collector_thread(*st);
    rrddim_set_by_pointer(*st, *rd, (collected_number)value);
    rrdset_done(*st);
}

static void msmq_queues_evict_unseen(usec_t now_ut)
{
    struct msmq_queue *mq;
    dfe_start_write(msmq_queues, mq)
    {
        if (mq->last_collected != now_ut)
            dictionary_del(msmq_queues, mq_dfe.name);
    }
    dfe_done(mq);
    dictionary_garbage_collect(msmq_queues);
}

static void msmq_queues_evict_all(void)
{
    struct msmq_queue *mq;
    dfe_start_write(msmq_queues, mq)
    {
        dictionary_del(msmq_queues, mq_dfe.name);
    }
    dfe_done(mq);
    dictionary_garbage_collect(msmq_queues);
}

static void msmq_other_charts_obsolete(void)
{
    rrdset_is_obsolete___safe_from_collector_thread(other_st_messages);
    rrdset_is_obsolete___safe_from_collector_thread(other_st_bytes);
    rrdset_is_obsolete___safe_from_collector_thread(other_st_journal_messages);
    rrdset_is_obsolete___safe_from_collector_thread(other_st_journal_bytes);
}

static void do_msmq(PERF_DATA_BLOCK *pDataBlock, int update_every)
{
    usec_t now_ut = now_monotonic_usec();
    PERF_OBJECT_TYPE *pObjectType = perflibFindObjectTypeByName(pDataBlock, "MSMQ Queue");
    if (!pObjectType) {
        // The query succeeded without the object, so no queues remain in this snapshot.
        msmq_queues_evict_all();
        msmq_other_charts_obsolete();
        return;
    }

    MSMQ_SAMPLE *samples = callocz(pObjectType->NumInstances > 0 ? (size_t)pObjectType->NumInstances : 1, sizeof(*samples));
    size_t samples_count = 0;
    size_t aggregated_queues = 0;
    bool complete = true;
    PERF_INSTANCE_DEFINITION *pi = NULL;
    for (LONG i = 0; i < pObjectType->NumInstances; i++) {
        pi = perflibForEachInstance(pDataBlock, pObjectType, pi);
        if (!pi) {
            complete = false;
            break;
        }

        if (!getInstanceName(pDataBlock, pObjectType, pi, windows_shared_buffer, sizeof(windows_shared_buffer))) {
            // Keep valid queue data from this cycle, but do not evict from an incomplete snapshot.
            complete = false;
            continue;
        }

        if (strcasecmp(windows_shared_buffer, "_Total") == 0)
            continue;

        DICT_ITEM_CONST DICTIONARY_ITEM *item =
            dictionary_set_and_acquire_item(msmq_queues, windows_shared_buffer, NULL, sizeof(struct msmq_queue));
        if (!item) {
            complete = false;
            continue;
        }

        struct msmq_queue *mq = dictionary_acquired_item_value(item);
        mq->last_collected = now_ut;
        mq->messages_in_queue.updated = false;
        mq->bytes_in_queue.updated = false;
        mq->messages_in_journal_queue.updated = false;
        mq->bytes_in_journal_queue.updated = false;

        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->messages_in_queue);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->bytes_in_queue);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->messages_in_journal_queue);
        perflibGetInstanceCounter(pDataBlock, pObjectType, pi, &mq->bytes_in_journal_queue);

        samples[samples_count++] = (MSMQ_SAMPLE){ .item = item, .mq = mq };
    }

    // Sort before applying the cap so dictionary enumeration order cannot change chart selection.
    qsort(samples, samples_count, sizeof(*samples), msmq_sample_compare);

    size_t individual = 0;
    uint64_t other_messages = 0, other_bytes = 0, other_journal_messages = 0, other_journal_bytes = 0;
    bool other_messages_ok = complete, other_bytes_ok = complete;
    bool other_journal_messages_ok = complete, other_journal_bytes_ok = complete;
    for (size_t i = 0; i < samples_count; i++) {
        MSMQ_SAMPLE *s = &samples[i];
        struct msmq_queue *mq = s->mq;
        const char *queue = dictionary_acquired_item_name(s->item);
        if (simple_pattern_matches(queues_to_chart, queue) && individual < (size_t)max_individual_queues) {
            individual++;
            if (mq->messages_in_queue.updated) msmq_queue_chart_messages(mq, queue, update_every);
            if (mq->bytes_in_queue.updated) msmq_queue_chart_bytes(mq, queue, update_every);
            if (mq->messages_in_journal_queue.updated) msmq_queue_chart_journal_messages(mq, queue, update_every);
            if (mq->bytes_in_journal_queue.updated) msmq_queue_chart_journal_bytes(mq, queue, update_every);
        } else {
            msmq_queue_obsolete_charts(mq);
            aggregated_queues++;
            if (mq->messages_in_queue.updated) other_messages += mq->messages_in_queue.current.Data; else other_messages_ok = false;
            if (mq->bytes_in_queue.updated) other_bytes += mq->bytes_in_queue.current.Data; else other_bytes_ok = false;
            if (mq->messages_in_journal_queue.updated) other_journal_messages += mq->messages_in_journal_queue.current.Data; else other_journal_messages_ok = false;
            if (mq->bytes_in_journal_queue.updated) other_journal_bytes += mq->bytes_in_journal_queue.current.Data; else other_journal_bytes_ok = false;
        }
        dictionary_acquired_item_release(msmq_queues, s->item);
    }
    freez(samples);

    // The static Other charts are created lazily and obsolete when no queue is aggregated.
    if (aggregated_queues) {
        if (other_messages_ok)
            msmq_chart_other(
                &other_st_messages,
                &other_rd_messages,
                "msmq_queue_messages",
                "msmq_queue_aggregate_messages_total",
                "queues",
                "msmq.queue_messages",
                "MSMQ messages in other queues",
                "messages",
                PRIO_MSMQ_QUEUE_MESSAGES,
                other_messages,
                update_every);
        if (other_bytes_ok)
            msmq_chart_other(
                &other_st_bytes,
                &other_rd_bytes,
                "msmq_queue_bytes",
                "msmq_queue_aggregate_bytes_total",
                "queues",
                "msmq.queue_bytes",
                "MSMQ bytes in other queues",
                "bytes",
                PRIO_MSMQ_QUEUE_BYTES,
                other_bytes,
                update_every);
        if (other_journal_messages_ok)
            msmq_chart_other(
                &other_st_journal_messages,
                &other_rd_journal_messages,
                "msmq_queue_journal_messages",
                "msmq_queue_aggregate_journal_messages_total",
                "journal",
                "msmq.queue_journal_messages",
                "MSMQ messages in other journal queues",
                "messages",
                PRIO_MSMQ_JOURNAL_MESSAGES,
                other_journal_messages,
                update_every);
        if (other_journal_bytes_ok)
            msmq_chart_other(
                &other_st_journal_bytes,
                &other_rd_journal_bytes,
                "msmq_queue_journal_bytes",
                "msmq_queue_aggregate_journal_bytes_total",
                "journal",
                "msmq.queue_journal_bytes",
                "MSMQ bytes in other journal queues",
                "bytes",
                PRIO_MSMQ_JOURNAL_BYTES,
                other_journal_bytes,
                update_every);
    } else if (complete) {
        msmq_other_charts_obsolete();
    }

    // A failed name decode still allows fresh values, but cannot prove which queues disappeared.
    if (complete)
        msmq_queues_evict_unseen(now_ut);

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
        return 0;

    do_msmq(pDataBlock, update_every);

    return 0;
}

void do_PerflibMSMQ_cleanup(void)
{
    dictionary_destroy(msmq_queues);
    msmq_queues = NULL;
    simple_pattern_free(queues_to_chart);
    queues_to_chart = NULL;
    msmq_other_charts_obsolete();
}
