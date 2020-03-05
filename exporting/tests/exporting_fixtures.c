// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"

int setup_configured_engine(void **state)
{
    struct engine *engine = __mock_read_exporting_config();

    *state = engine;

    return 0;
}

int teardown_configured_engine(void **state)
{
    struct engine *engine = *state;

    struct instance *instance = engine->instance_root;
    free((void *)instance->config.destination);
    free((void *)instance->config.name);
    simple_pattern_free(instance->config.charts_pattern);
    simple_pattern_free(instance->config.hosts_pattern);
    free(instance);

    free((void *)engine->config.prefix);
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
    localhost->labels = label;

    label = calloc(1, sizeof(struct label));
    label->key = strdupz("key2");
    label->value = strdupz("value2");
    label->label_source = LABEL_SOURCE_AUTO;
    localhost->labels->next = label;

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

    free(localhost->labels->next->key);
    free(localhost->labels->next->value);
    free(localhost->labels->next);
    free(localhost->labels->key);
    free(localhost->labels->value);
    free(localhost->labels);

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
