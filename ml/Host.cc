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

void TrainableHost::trainOne(TimePoint &Now) {
    for (auto &DP : DimensionsMap) {
        Dimension *D = DP.second;

        MLResult Result = D->trainModel(Now);

        switch (Result) {
        case MLResult::Success:
            error("Trained dimension: %s\n", D->getRD()->id);
            return;
        case MLResult::TryLockFailed:
        case MLResult::ShouldNotTrainNow:
        case MLResult::MissingData:
            continue;
        default:
            error("Unhandled MLError enumeration value");
        }
    }
}

void TrainableHost::train() {
    while (!netdata_exit) {
        size_t NumDimensions;

        TimePoint StartTP = SteadyClock::now();
        {
            std::lock_guard<std::mutex> Lock(Mutex);

            NumDimensions = DimensionsMap.size();
            trainOne(StartTP);
        }
        Duration<double> RealDuration = SteadyClock::now() - StartTP;
        Duration<double> AllottedDuration = Duration<double>{Cfg.TrainEvery} / (NumDimensions + 1);

        if (RealDuration >= AllottedDuration)
            continue;

        Duration<double> SleepFor = AllottedDuration - RealDuration;
        Duration<double> MaxSleepFor = Seconds{1};
        std::this_thread::sleep_for(std::min(SleepFor, MaxSleepFor));
    }
}

void TrainableHost::startTrainingThread() {
    TrainingThread = std::thread(&TrainableHost::train, this);
}

void TrainableHost::stopTrainingThread() {
    TrainingThread.join();
}
