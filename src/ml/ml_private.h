// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_PRIVATE_H
#define NETDATA_ML_PRIVATE_H

#include <vector>
#include <unordered_map>

#include "ml_config.h"

void ml_train_main(void *arg);
void ml_detect_main(void *arg);

bool ml_dimension_train_model_precheck(enum ml_metric_type mt,
                                       bool has_received_downstream_model,
                                       bool training_in_progress,
                                       enum ml_worker_result *worker_res);
bool ml_should_requeue_create_new_model(enum ml_worker_result worker_res);
bool ml_should_publish_model_update(bool host_running,
                                    uint32_t current_generation,
                                    uint32_t expected_generation,
                                    bool *training_in_progress);

extern sqlite3 *ml_db;
extern const char *db_models_create_table;


#endif /* NETDATA_ML_PRIVATE_H */
