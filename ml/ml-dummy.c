// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml.h"

#if !defined(ENABLE_ML)

bool ml_capable() {
    return false;
}

bool ml_enabled(RRDHOST *rh) {
    UNUSED(rh);
    return false;
}

bool ml_streaming_enabled() {
    return false;
}

void ml_init(void) {}

void ml_fini(void) {}

void ml_start_threads(void) {}

void ml_stop_threads(void) {}

void ml_host_new(RRDHOST *rh) {
    UNUSED(rh);
}

void ml_host_delete(RRDHOST *rh) {
    UNUSED(rh);
}

void ml_host_start_training_thread(RRDHOST *rh) {
    UNUSED(rh);
}

void ml_host_stop_training_thread(RRDHOST *rh) {
    UNUSED(rh);
}

void ml_host_cancel_training_thread(RRDHOST *rh) {
    UNUSED(rh);
}

void ml_host_get_info(RRDHOST *rh, BUFFER *wb) {
    UNUSED(rh);
    UNUSED(wb);
}

void ml_host_get_models(RRDHOST *rh, BUFFER *wb) {
    UNUSED(rh);
    UNUSED(wb);
}

void ml_host_get_runtime_info(RRDHOST *rh) {
    UNUSED(rh);
}

void ml_chart_new(RRDSET *rs) {
    UNUSED(rs);
}

void ml_chart_delete(RRDSET *rs) {
    UNUSED(rs);
}

bool ml_chart_update_begin(RRDSET *rs) {
    UNUSED(rs);
    return false;
}

void ml_chart_update_end(RRDSET *rs) {
    UNUSED(rs);
}

void ml_dimension_new(RRDDIM *rd) {
    UNUSED(rd);
}

void ml_dimension_delete(RRDDIM *rd) {
    UNUSED(rd);
}

bool ml_dimension_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists) {
    UNUSED(rd);
    UNUSED(curr_time);
    UNUSED(value);
    UNUSED(exists);
    return false;
}

int ml_dimension_load_models(RRDDIM *rd) {
    UNUSED(rd);
    return 0;
}

void ml_update_global_statistics_charts(uint64_t models_consulted) {
    UNUSED(models_consulted);
}

#endif
