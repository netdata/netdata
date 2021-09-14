// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Host.h"
#include "Dimension.h"

using namespace ml;

void RrdHost::addDimension(Dimension *D) {
    std::lock_guard<std::mutex> Lock(Mutex);
    DimensionsMap[D->getRD()] = D;
}

void RrdHost::removeDimension(Dimension *D) {
    std::lock_guard<std::mutex> Lock(Mutex);
    DimensionsMap.erase(D->getRD());
}
