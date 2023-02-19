// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_HOST_H
#define ML_HOST_H

#include "Mutex.h"
#include "Config.h"
#include "Dimension.h"
#include "Chart.h"
#include "Queue.h"

#include "ml-private.h"
#include "json/single_include/nlohmann/json.hpp"

namespace ml
{

class Host {

friend void* train_main(void *);
friend void *detect_main(void *);

public:
    Host(RRDHOST *RH) :
        RH(RH),
        MLS(),
        TS(),
        HostAnomalyRate(0.0),
        ThreadsRunning(false),
        ThreadsCancelled(false),
        ThreadsJoined(false)
        {}

    void addChart(Chart *C);
    void removeChart(Chart *C);

    void getConfigAsJson(BUFFER *wb) const;
    void getModelsAsJson(nlohmann::json &Json);
    void getDetectionInfoAsJson(nlohmann::json &Json) const;

    void startAnomalyDetectionThreads();
    void stopAnomalyDetectionThreads(bool join);

    void scheduleForTraining(TrainingRequest TR);
    void train();

    void detect();
    void detectOnce();

private:
    RRDHOST *RH;
    MachineLearningStats MLS;
    TrainingStats TS;
    CalculatedNumber HostAnomalyRate{0.0};
    std::atomic<bool> ThreadsRunning;
    std::atomic<bool> ThreadsCancelled;
    std::atomic<bool> ThreadsJoined;

    Queue<TrainingRequest> TrainingQueue;

    Mutex M;
    std::unordered_map<RRDSET *, Chart *> Charts;

    netdata_thread_t TrainingThread;
    netdata_thread_t DetectionThread;
};

} // namespace ml

#endif /* ML_HOST_H */
