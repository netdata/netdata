// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_HOST_H
#define ML_HOST_H

#include "ml-private.h"

#include "Dimension.h"

namespace ml {

class RrdHost {
public:
    RrdHost(RRDHOST *RH) : RH(RH) {}

    void addDimension(Dimension *D);
    void removeDimension(Dimension *D);

private:
    RRDHOST *RH;

    std::mutex Mutex;
    std::map<RRDDIM *, Dimension *> DimensionsMap;
};

using Host = RrdHost;

} // namespace ml

#endif /* ML_HOST_H */
