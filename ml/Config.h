// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_CONFIG_H
#define ML_CONFIG_H

#include "ml-private.h"

namespace ml {

class Config {
public:
    SIMPLE_PATTERN *SP_HostsToSkip;
    SIMPLE_PATTERN *SP_ChartsToSkip;

    void readMLConfig();
};

extern Config Cfg;

} // namespace ml

#endif /* ML_CONFIG_H */
