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
                                    bool superseded_by_downstream,
                                    bool *training_in_progress);

// Whether the downstream model `km` may be installed on `dim`: not a duplicate and newer than the newest installed
// model. A local training in progress does not block it. The caller holds dim->slock and installs in the same
// critical section.
bool ml_dimension_accept_downstream_model(const ml_dimension_t *dim, const ml_kmeans_inlined_t &km);

// One create-model step of a training thread: acquire the dimension and train it. `worker` is used only once the
// dimension is acquired.
enum ml_worker_result ml_worker_create_new_model(ml_worker_t *worker, const ml_request_create_new_model_t &req);

// The models installed by any training thread and not yet written to ml.db (Cfg.pending_models).
void ml_pending_models_add(const ml_model_info_t &model_info);
size_t ml_pending_models_count();
// Moves every pending model, in install order, into `batch` (cleared first).
void ml_pending_models_take(std::vector<ml_model_info_t> &batch);

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

// Force the "ml.db unusable" flag on without dropping a sentinel or
// writing to disk. Used at startup when we know ml.db is poisoned
// (e.g., a sentinel from a prior session exists but couldn't be
// consumed) and want to skip opening the bad DB for this session.
void ml_db_force_unusable(void);


#endif /* NETDATA_ML_PRIVATE_H */
