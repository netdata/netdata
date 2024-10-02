// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ND_OTEL_TRANSFORM_H
#define ND_OTEL_TRANSFORM_H

#include "otel_utils.h"
#include "otel_config.h"

namespace pb
{
    void transformResourceMetrics(const otel::Config *Cfg, pb::RepeatedPtrField<pb::ResourceMetrics> &RPF);
} // namespace pb

#endif /* ND_OTEL_TRANSFORM_H */
