// SPDX-License-Identifier: GPL-3.0-or-later


#ifndef NETDATA_EXPORTING_GRAPHITE_H
#define NETDATA_EXPORTING_GRAPHITE_H

#include "exporting/exporting_engine.h"

int exporting_format_dimension_collected_graphite_plaintext(struct engine *engine);
int init_graphite_instance(struct instance *instance);

#endif //NETDATA_EXPORTING_GRAPHITE_H
