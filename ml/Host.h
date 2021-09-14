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

    virtual ~RrdHost() {};

protected:
    RRDHOST *RH;

    std::mutex Mutex;
    std::map<RRDDIM *, Dimension *> DimensionsMap;
};

class TrainableHost : public RrdHost {
public:
    TrainableHost(RRDHOST *RH) : RrdHost(RH) {}

    void startTrainingThread();
    void stopTrainingThread();

private:
    void train();
    void trainOne(TimePoint &Now);

private:
    std::thread TrainingThread;
};

using Host = TrainableHost;

} // namespace ml

#endif /* ML_HOST_H */
