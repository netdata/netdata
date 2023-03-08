// SPDX-License-Identifier: GPL-3.0-or-later

#include "nml.h"

#include <random>

bool ml_capable() {
    return true;
}

bool ml_enabled(RRDHOST *rh) {
    if (!Cfg.enable_anomaly_detection)
        return false;

    if (simple_pattern_matches(Cfg.sp_host_to_skip, rrdhost_hostname(rh)))
        return false;

    return true;
}

/*
 * Assumptions:
 *  1) hosts outlive their sets, and sets outlive their dimensions,
 *  2) dimensions always have a set that has a host.
 */

void ml_init(void) {
    // Read config values
    nml_config_load(&Cfg);

    if (!Cfg.enable_anomaly_detection)
        return;

    // Generate random numbers to efficiently sample the features we need
    // for KMeans clustering.
    std::random_device RD;
    std::mt19937 Gen(RD());

    Cfg.random_nums.reserve(Cfg.max_train_samples);
    for (size_t Idx = 0; Idx != Cfg.max_train_samples; Idx++)
        Cfg.random_nums.push_back(Gen());


    // start detection & training threads
    char tag[NETDATA_THREAD_TAG_MAX + 1];

    snprintfz(tag, NETDATA_THREAD_TAG_MAX, "%s", "PREDICT");
    netdata_thread_create(&Cfg.detection_thread, tag, NETDATA_THREAD_OPTION_JOINABLE, nml_detect_main, NULL);
}

void ml_host_new(RRDHOST *rh) {
    if (!ml_enabled(rh))
        return;

    nml_host_t *host = nml_host_new(rh);
    rh->ml_host = reinterpret_cast<ml_host_t *>(host);
}

void ml_host_delete(RRDHOST *rh) {
    nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
    if (!host)
        return;

    nml_host_delete(host);
    rh->ml_host = NULL;
}

void ml_chart_new(RRDSET *rs) {
    nml_host_t *host = reinterpret_cast<nml_host_t *>(rs->rrdhost->ml_host);
    if (!host)
        return;

    nml_chart_t *chart = nml_chart_new(rs);
    rs->ml_chart = reinterpret_cast<ml_chart_t *>(chart);
}

void ml_chart_delete(RRDSET *rs) {
    nml_host_t *host = reinterpret_cast<nml_host_t *>(rs->rrdhost->ml_host);
    if (!host)
        return;

    nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rs->ml_chart);

    nml_chart_delete(chart);
    rs->ml_chart = NULL;
}

void ml_dimension_new(RRDDIM *rd) {
    nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rd->rrdset->ml_chart);
    if (!chart)
        return;

    nml_dimension_t *dim = nml_dimension_new(rd);
    rd->ml_dimension = reinterpret_cast<ml_dimension_t *>(dim);
}

void ml_dimension_delete(RRDDIM *rd) {
    nml_dimension_t *dim = reinterpret_cast<nml_dimension_t *>(rd->ml_dimension);
    if (!dim)
        return;

    nml_dimension_delete(dim);
    rd->ml_dimension = NULL;
}

void ml_get_host_info(RRDHOST *rh, BUFFER *wb) {
    if (rh && rh->ml_host) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
        nml_host_get_config_as_json(host, wb);
    } else {
        buffer_json_member_add_boolean(wb, "enabled", false);
    }
}

char *ml_get_host_runtime_info(RRDHOST *rh) {
    nlohmann::json config_json;

    if (rh && rh->ml_host) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
        nml_host_get_detection_info_as_json(host, config_json);
    } else {
        return NULL;
    }

    return strdup(config_json.dump(1, '\t').c_str());
}

char *ml_get_host_models(RRDHOST *rh) {
    nlohmann::json j;

    if (rh && rh->ml_host) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
        nml_host_get_models_as_json(host, j);
        return strdup(j.dump(2, '\t').c_str());
    }

    return NULL;
}

bool ml_chart_update_begin(RRDSET *rs) {
    nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rs->ml_chart);
    if (!chart)
        return false;

    nml_chart_update_begin(chart);
    return true;
}

void ml_chart_update_end(RRDSET *rs) {
    nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rs->ml_chart);
    if (!chart)
        return;

    nml_chart_update_end(chart);
}

bool ml_is_anomalous(RRDDIM *rd, time_t curr_time, double value, bool exists) {
    nml_dimension_t *dim = reinterpret_cast<nml_dimension_t *>(rd->ml_dimension);
    if (!dim)
        return false;

    nml_chart_t *chart = reinterpret_cast<nml_chart_t *>(rd->rrdset->ml_chart);

    bool is_anomalous = nml_dimension_predict(dim, curr_time, value, exists);
    nml_chart_update_dimension(chart, dim, is_anomalous);

    return is_anomalous;
}

bool ml_streaming_enabled() {
    return Cfg.stream_anomaly_detection_charts;
}

void ml_start_training_thread(RRDHOST *rh) {
    if (rh && rh->ml_host) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
        nml_host_start_training_thread(host);
    }
}

void ml_stop_training_thread(RRDHOST *rh) {
    if (rh && rh->ml_host) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
        nml_host_stop_training_thread(host, /* join */ true);
    }
}

void ml_cancel_training_thread(RRDHOST *rh) {
    if (rh && rh->ml_host) {
        nml_host_t *host = reinterpret_cast<nml_host_t *>(rh->ml_host);
        nml_host_stop_training_thread(host, /* join */ false);
    }
}
