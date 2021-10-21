// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"

int setup_configured_engine(void **state)
{
    struct engine *engine = __mock_read_exporting_config();
    engine->instance_root->data_is_ready = 1;

    *state = engine;

    return 0;
}

int teardown_configured_engine(void **state)
{
    struct engine *engine = *state;

    struct instance *instance = engine->instance_root;
    free((void *)instance->config.destination);
    free((void *)instance->config.username);
    free((void *)instance->config.password);
    free((void *)instance->config.name);
    free((void *)instance->config.prefix);
    free((void *)instance->config.hostname);
    simple_pattern_free(instance->config.charts_pattern);
    simple_pattern_free(instance->config.hosts_pattern);
    free(instance);

    free((void *)engine->config.hostname);
    free(engine);

    return 0;
}

int setup_rrdhost()
{
    localhost = calloc(1, sizeof(RRDHOST));

    localhost->rrd_update_every = 1;

    localhost->tags = strdupz("TAG1=VALUE1 TAG2=VALUE2");

    struct label *label = calloc(1, sizeof(struct label));
    label->key = strdupz("key1");
    label->value = strdupz("value1");
    label->label_source = LABEL_SOURCE_NETDATA_CONF;
    localhost->labels.head = label;

    label = calloc(1, sizeof(struct label));
    label->key = strdupz("key2");
    label->value = strdupz("value2");
    label->label_source = LABEL_SOURCE_AUTO;
    localhost->labels.head->next = label;

    localhost->rrdset_root = calloc(1, sizeof(RRDSET));
    RRDSET *st = localhost->rrdset_root;
    st->rrdhost = localhost;
    strcpy(st->id, "chart_id");
    st->name = strdupz("chart_name");
    st->flags |= RRDSET_FLAG_ENABLED;
    st->rrd_memory_mode |= RRD_MEMORY_MODE_SAVE;
    st->update_every = 1;

    localhost->rrdset_root->dimensions = calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    rd->rrdset = st;
    rd->id = strdupz("dimension_id");
    rd->name = strdupz("dimension_name");
    rd->last_collected_value = 123000321;
    rd->last_collected_time.tv_sec = 15051;
    rd->collections_counter++;
    rd->next = NULL;

    rd->state = calloc(1, sizeof(*rd->state));
    rd->state->query_ops.oldest_time = __mock_rrddim_query_oldest_time;
    rd->state->query_ops.latest_time = __mock_rrddim_query_latest_time;
    rd->state->query_ops.init = __mock_rrddim_query_init;
    rd->state->query_ops.is_finished = __mock_rrddim_query_is_finished;
    rd->state->query_ops.next_metric = __mock_rrddim_query_next_metric;
    rd->state->query_ops.finalize = __mock_rrddim_query_finalize;

    return 0;
}

int teardown_rrdhost()
{
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    free((void *)rd->name);
    free((void *)rd->id);
    free(rd->state);
    free(rd);

    RRDSET *st = localhost->rrdset_root;
    free((void *)st->name);
    free(st);

    free(localhost->labels.head->next->key);
    free(localhost->labels.head->next->value);
    free(localhost->labels.head->next);
    free(localhost->labels.head->key);
    free(localhost->labels.head->value);
    free(localhost->labels.head);

    free((void *)localhost->tags);
    free(localhost);

    return 0;
}

int setup_initialized_engine(void **state)
{
    setup_configured_engine(state);

    struct engine *engine = *state;
    init_connectors_in_tests(engine);

    setup_rrdhost();

    return 0;
}

int teardown_initialized_engine(void **state)
{
    struct engine *engine = *state;

    teardown_rrdhost();
    buffer_free(engine->instance_root->labels);
    buffer_free(engine->instance_root->buffer);
    teardown_configured_engine(state);

    return 0;
}

int setup_prometheus(void **state)
{
    (void)state;

    prometheus_exporter_instance = calloc(1, sizeof(struct instance));

    setup_rrdhost();

    prometheus_exporter_instance->config.update_every = 10;

    prometheus_exporter_instance->config.options |=
        EXPORTING_OPTION_SEND_NAMES | EXPORTING_OPTION_SEND_CONFIGURED_LABELS | EXPORTING_OPTION_SEND_AUTOMATIC_LABELS;

    prometheus_exporter_instance->config.charts_pattern = simple_pattern_create("*", NULL, SIMPLE_PATTERN_EXACT);
    prometheus_exporter_instance->config.hosts_pattern = simple_pattern_create("*", NULL, SIMPLE_PATTERN_EXACT);

    prometheus_exporter_instance->config.initialized = 1;

    return 0;
}

int teardown_prometheus(void **state)
{
    (void)state;

    teardown_rrdhost();

    simple_pattern_free(prometheus_exporter_instance->config.charts_pattern);
    simple_pattern_free(prometheus_exporter_instance->config.hosts_pattern);
    free(prometheus_exporter_instance);

    return 0;
}
