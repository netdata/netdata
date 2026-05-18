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

// Mark ml.db as corrupt: drops a `.ml.db.delete` sentinel in the cache dir
// (best-effort, idempotent) and latches the "ml.db unusable" flag so
// subsequent ml.db access short-circuits for the rest of the session. The
// sentinel is consumed at next startup, which renames
// ml.db -> ml.db.bad.<usec-timestamp> and creates a fresh DB.
// `rc` is the SQLite error code that triggered the call; logged raw for
// diagnostics. It may be a primary code (SQLITE_CORRUPT, SQLITE_NOTADB) or
// an extended variant (SQLITE_CORRUPT_VTAB, SQLITE_CORRUPT_INDEX, ...).
// Callers should normally route through ml_db_mark_if_corrupt() rather than
// invoking this directly.
void ml_db_mark_corrupt(int rc);

// Flag ml.db as corrupt if `rc` is a corruption signal (SQLITE_CORRUPT or
// SQLITE_NOTADB, including extended variants which encode the primary code
// in the low 8 bits). Returns true when `rc` indicated corruption,
// regardless of whether the sentinel was successfully written.
bool ml_db_mark_if_corrupt(int rc);

// True if ml.db has been flagged unusable in this session by an earlier
// CORRUPT/NOTADB detection. The flag is read/written via __atomic_* on the
// underlying storage; access only through this accessor to keep the
// atomic contract self-enforcing.
bool ml_db_is_unusable(void);


#endif /* NETDATA_ML_PRIVATE_H */
