// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_TRANSFORM_H
#define ND_OTEL_TRANSFORM_H

#include "otel_utils.h"
#include "otel_config.h"

namespace pb
{
void transformMetricData(const otel::Config *Cfg, pb::MetricsData &MD);
} // namespace pb

#endif /* ND_OTEL_TRANSFORM_H */
