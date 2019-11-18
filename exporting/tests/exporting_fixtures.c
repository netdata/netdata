// SPDX-License-Identifier: GPL-3.0-or-later

#include "test_exporting_engine.h"

int setup_configured_engine(void **state)
{
    struct engine *engine = __mock_read_exporting_config();

    engine->after = 1;
    engine->before = 2;

    *state = engine;

    return 0;
}

int teardown_configured_engine(void **state)
{
    struct engine *engine = *state;

    struct instance *instance = engine->connector_root->instance_root;
    free((void *)instance->config.destination);
    free(instance->config.charts_pattern);
    free(instance->config.hosts_pattern);
    free(instance);

    free(engine->connector_root);

    free((void *)engine->config.prefix);
    free((void *)engine->config.hostname);
    free(engine);

    return 0;
}

int setup_rrdhost()
{
    localhost = calloc(1, sizeof(RRDHOST));
    localhost->tags = strdupz("TAG1=VALUE1 TAG2=VALUE2");

    localhost->rrdset_root = calloc(1, sizeof(RRDSET));
    RRDSET *st = localhost->rrdset_root;
    st->rrdhost = localhost;
    strcpy(st->id, "chart_id");
    st->name = strdupz("chart_name");

    localhost->rrdset_root->dimensions = calloc(1, sizeof(RRDDIM));
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    rd->rrdset = st;
    rd->id = strdupz("dimension_id");
    rd->name = strdupz("dimension_name");
    rd->last_collected_value = 123000321;
    rd->last_collected_time.tv_sec = 15051;
    rd->next = NULL;

    return 0;
}

int teardown_rrdhost()
{
    RRDDIM *rd = localhost->rrdset_root->dimensions;
    free((void *)rd->name);
    free((void *)rd->id);
    free(rd);

    RRDSET *st = localhost->rrdset_root;
    free((void *)st->name);
    free(st);

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
    teardown_rrdhost();
    teardown_configured_engine(state);

    return 0;
}
