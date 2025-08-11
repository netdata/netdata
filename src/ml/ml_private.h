// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_ML_PRIVATE_H
#define NETDATA_ML_PRIVATE_H

#include <vector>
#include <unordered_map>

#include "ml_config.h"

void ml_train_main(void *arg);
void ml_detect_main(void *arg);

extern sqlite3 *ml_db;
extern const char *db_models_create_table;


#endif /* NETDATA_ML_PRIVATE_H */
