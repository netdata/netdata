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

    localhost->tags = string_strdupz("TAG1=VALUE1 TAG2=VALUE2");

    localhost->rrdlabels = rrdlabels_create();
    rrdlabels_add(localhost->rrdlabels, "key1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(localhost->rrdlabels, "key2", "value2", RRDLABEL_SRC_CONFIG);

    localhost->rrdset_root = calloc(1, sizeof(RRDSET));
    RRDSET *st = localhost->rrdset_root;
    st->rrdhost = localhost;
    st->id = string_strdupz("chart_id");
    st->name = string_strdupz("chart_name");
    st->rrd_memory_mode |= RRD_MEMORY_MODE_SAVE;
    st->update_every = 1;

    localhost->rrdset_root->dimensions = calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    rd->rrdset = st;
    rd->id = string_strdupz("dimension_id");
    rd->name = string_strdupz("dimension_name");
    rd->last_collected_value = 123000321;
    rd->last_collected_time.tv_sec = 15051;
    rd->collections_counter++;
    rd->next = NULL;

    rd->tiers[0] = calloc(1, sizeof(struct rrddim_tier));
    rd->tiers[0]->query_ops.oldest_time = __mock_rrddim_query_oldest_time;
    rd->tiers[0]->query_ops.latest_time = __mock_rrddim_query_latest_time;
    rd->tiers[0]->query_ops.init = __mock_rrddim_query_init;
    rd->tiers[0]->query_ops.is_finished = __mock_rrddim_query_is_finished;
    rd->tiers[0]->query_ops.next_metric = __mock_rrddim_query_next_metric;
    rd->tiers[0]->query_ops.finalize = __mock_rrddim_query_finalize;

    return 0;
}

int teardown_rrdhost()
{
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    string_freez(rd->name);
    string_freez(rd->id);
    free(rd->tiers[0]);
    free(rd);

    RRDSET *st = localhost->rrdset_root;
    string_freez(st->name);
    free(st);

    rrdlabels_destroy(localhost->rrdlabels);

    string_freez(localhost->tags);
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
    buffer_free(engine->instance_root->labels_buffer);
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
