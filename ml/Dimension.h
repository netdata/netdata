// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DIMENSION_H
#define ML_DIMENSION_H

#include "ml-private.h"

namespace ml {

class RrdDimension {
public:
    RrdDimension(RRDDIM *RD) : RD(RD), Ops(&RD->state->query_ops) {}

    RRDDIM *getRD() const { return RD; }

private:
    RRDDIM *RD;
    struct rrddim_volatile::rrddim_query_ops *Ops;
};

using Dimension = RrdDimension;

} // namespace ml

#endif /* ML_DIMENSION_H */
