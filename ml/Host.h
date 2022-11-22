// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_HOST_H
#define ML_HOST_H

#include "Config.h"
#include "Dimension.h"
#include "Chart.h"
#include "Queue.h"

#include "ml-private.h"
#include "json/single_include/nlohmann/json.hpp"

namespace ml
{

class Host {
public:
    Host(RRDHOST *RH) :
        RH(RH),
        MLS(),
        TS(),
        HostAnomalyRate(0.0)
    { }

    void addChart(Chart *C);
    void removeChart(Chart *C);

    void getConfigAsJson(nlohmann::json &Json) const;
    void getModelsAsJson(nlohmann::json &Json);
    void getDetectionInfoAsJson(nlohmann::json &Json) const;

    void startAnomalyDetectionThreads();
    void stopAnomalyDetectionThreads();

    void scheduleForTraining(TrainingRequest TR);
    void train();

    void detect();
    void detectOnce();

private:
    RRDHOST *RH;
    MachineLearningStats MLS;
    TrainingStats TS;
    CalculatedNumber HostAnomalyRate{0.0};

    Queue<TrainingRequest> TrainingQueue;

    std::mutex Mutex;
    std::unordered_map<RRDSET *, Chart *> Charts;

    std::thread TrainingThread;
    std::thread DetectionThread;
};

} // namespace ml

#endif /* ML_HOST_H */
