// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Host.h"
#include "Queue.h"
#include "ADCharts.h"

#include "json/single_include/nlohmann/json.hpp"

using namespace ml;

void Host::addChart(Chart *C) {
    std::lock_guard<Mutex> L(M);
    Charts[C->getRS()] = C;
}

void Host::removeChart(Chart *C) {
    std::lock_guard<Mutex> L(M);
    Charts.erase(C->getRS());
}

void Host::getConfigAsJson(nlohmann::json &Json) const {
    Json["version"] = 1;

    Json["enabled"] = Cfg.EnableAnomalyDetection;

    Json["min-train-samples"] = Cfg.MinTrainSamples;
    Json["max-train-samples"] = Cfg.MaxTrainSamples;
    Json["train-every"] = Cfg.TrainEvery;

    Json["diff-n"] = Cfg.DiffN;
    Json["smooth-n"] = Cfg.SmoothN;
    Json["lag-n"] = Cfg.LagN;

    Json["random-sampling-ratio"] = Cfg.RandomSamplingRatio;
    Json["max-kmeans-iters"] = Cfg.MaxKMeansIters;

    Json["dimension-anomaly-score-threshold"] = Cfg.DimensionAnomalyScoreThreshold;

    Json["host-anomaly-rate-threshold"] = Cfg.HostAnomalyRateThreshold;
    Json["anomaly-detection-grouping-method"] = group_method2string(Cfg.AnomalyDetectionGroupingMethod);
    Json["anomaly-detection-query-duration"] = Cfg.AnomalyDetectionQueryDuration;

    Json["hosts-to-skip"] = Cfg.HostsToSkip;
    Json["charts-to-skip"] = Cfg.ChartsToSkip;
}

void Host::getModelsAsJson(nlohmann::json &Json) {
    std::lock_guard<Mutex> L(M);

    for (auto &CP : Charts) {
        Chart *C = CP.second;
        C->getModelsAsJson(Json);
    }
}

void Host::detectOnce() {
    MLS = {};
    MachineLearningStats MLSCopy = {};
    TrainingStats TSCopy = {};

    {
        std::lock_guard<Mutex> L(M);

        /*
         * prediction/detection stats
        */
        for (auto &CP : Charts) {
            Chart *C = CP.second;

            if (!C->isAvailableForML())
                continue;

            MachineLearningStats ChartMLS = C->getMLS();

            MLS.NumMachineLearningStatusEnabled += ChartMLS.NumMachineLearningStatusEnabled;
            MLS.NumMachineLearningStatusDisabledUE += ChartMLS.NumMachineLearningStatusDisabledUE;
            MLS.NumMachineLearningStatusDisabledSP += ChartMLS.NumMachineLearningStatusDisabledSP;

            MLS.NumMetricTypeConstant += ChartMLS.NumMetricTypeConstant;
            MLS.NumMetricTypeVariable += ChartMLS.NumMetricTypeVariable;

            MLS.NumTrainingStatusUntrained += ChartMLS.NumTrainingStatusUntrained;
            MLS.NumTrainingStatusPendingWithoutModel += ChartMLS.NumTrainingStatusPendingWithoutModel;
            MLS.NumTrainingStatusTrained += ChartMLS.NumTrainingStatusTrained;
            MLS.NumTrainingStatusPendingWithModel += ChartMLS.NumTrainingStatusPendingWithModel;

            MLS.NumAnomalousDimensions += ChartMLS.NumAnomalousDimensions;
            MLS.NumNormalDimensions += ChartMLS.NumNormalDimensions;
        }

        HostAnomalyRate = 0.0;
        size_t NumActiveDimensions = MLS.NumAnomalousDimensions + MLS.NumNormalDimensions;
        if (NumActiveDimensions)
              HostAnomalyRate = static_cast<double>(MLS.NumAnomalousDimensions) / NumActiveDimensions;

        MLSCopy = MLS;

        /*
         * training stats
        */
        TSCopy = TS;

        TS.QueueSize = 0;
        TS.NumPoppedItems = 0;

        TS.AllottedUT = 0;
        TS.ConsumedUT = 0;
        TS.RemainingUT = 0;

        TS.TrainingResultOk = 0;
        TS.TrainingResultInvalidQueryTimeRange = 0;
        TS.TrainingResultNotEnoughCollectedValues = 0;
        TS.TrainingResultNullAcquiredDimension = 0;
        TS.TrainingResultChartUnderReplication = 0;
    }

    // Calc the avg values
    if (TSCopy.NumPoppedItems) {
        TSCopy.QueueSize /= TSCopy.NumPoppedItems;
        TSCopy.AllottedUT /= TSCopy.NumPoppedItems;
        TSCopy.ConsumedUT /= TSCopy.NumPoppedItems;
        TSCopy.RemainingUT /= TSCopy.NumPoppedItems;

        TSCopy.TrainingResultOk /= TSCopy.NumPoppedItems;
        TSCopy.TrainingResultInvalidQueryTimeRange /= TSCopy.NumPoppedItems;
        TSCopy.TrainingResultNotEnoughCollectedValues /= TSCopy.NumPoppedItems;
        TSCopy.TrainingResultNullAcquiredDimension /= TSCopy.NumPoppedItems;
        TSCopy.TrainingResultChartUnderReplication /= TSCopy.NumPoppedItems;
    } else {
        TSCopy.QueueSize = 0;
        TSCopy.AllottedUT = 0;
        TSCopy.ConsumedUT = 0;
        TSCopy.RemainingUT = 0;
    }

    updateDimensionsChart(RH, MLSCopy);
    updateHostAndDetectionRateCharts(RH, HostAnomalyRate * 10000.0);

    struct rusage PredictionRU;
    getrusage(RUSAGE_THREAD, &PredictionRU);
    updateResourceUsageCharts(RH, PredictionRU, TSCopy.TrainingRU);

    updateTrainingStatisticsChart(RH, TSCopy);
}

class AcquiredDimension {
public:
    static AcquiredDimension find(RRDHOST *RH, STRING *ChartId, STRING *DimensionId) {
        RRDDIM_ACQUIRED *AcqRD = nullptr;
        Dimension *D = nullptr;

        rrdhost_rdlock(RH);
        RRDSET *RS = rrdset_find(RH, string2str(ChartId));
        if (RS) {
            AcqRD = rrddim_find_and_acquire(RS, string2str(DimensionId));
            if (AcqRD) {
                RRDDIM *RD = rrddim_acquired_to_rrddim(AcqRD);
                if (RD)
                    D = reinterpret_cast<Dimension *>(RD->ml_dimension);
            }
        }
        rrdhost_unlock(RH);

        return AcquiredDimension(AcqRD, D);
    }

private:
    AcquiredDimension(RRDDIM_ACQUIRED *AcqRD, Dimension *D) : AcqRD(AcqRD), D(D) {}

public:
    TrainingResult train(const TrainingRequest &TR) {
        if (!D)
            return TrainingResult::NullAcquiredDimension;

        return D->trainModel(TR);
    }

    ~AcquiredDimension() {
        if (AcqRD)
            rrddim_acquired_release(AcqRD);
    }

private:
    RRDDIM_ACQUIRED *AcqRD;
    Dimension *D;
};

void Host::scheduleForTraining(TrainingRequest TR) {
    TrainingQueue.push(TR);
}

void Host::train() {
    while (!netdata_exit) {
        auto P = TrainingQueue.pop();
        TrainingRequest TrainingReq = P.first;
        size_t Size = P.second;

        usec_t AllottedUT = (Cfg.TrainEvery * RH->rrd_update_every * USEC_PER_SEC) / Size;
        if (AllottedUT > USEC_PER_SEC)
            AllottedUT = USEC_PER_SEC;

        usec_t StartUT = now_realtime_usec();
        TrainingResult TrainingRes;
        {
            AcquiredDimension AcqDim = AcquiredDimension::find(RH, TrainingReq.ChartId, TrainingReq.DimensionId);
            TrainingRes = AcqDim.train(TrainingReq);
            string_freez(TrainingReq.ChartId);
            string_freez(TrainingReq.DimensionId);
        }
        usec_t ConsumedUT = now_realtime_usec() - StartUT;

        usec_t RemainingUT = 0;
        if (ConsumedUT < AllottedUT)
            RemainingUT = AllottedUT - ConsumedUT;

        {
            std::lock_guard<Mutex> L(M);

            if (TS.AllottedUT == 0) {
                struct rusage TRU;
                getrusage(RUSAGE_THREAD, &TRU);
                TS.TrainingRU = TRU;
            }

            TS.QueueSize += Size;
            TS.NumPoppedItems += 1;

            TS.AllottedUT += AllottedUT;
            TS.ConsumedUT += ConsumedUT;
            TS.RemainingUT += RemainingUT;

            switch (TrainingRes) {
                case TrainingResult::Ok:
                    TS.TrainingResultOk += 1;
                    break;
                case TrainingResult::InvalidQueryTimeRange:
                    TS.TrainingResultInvalidQueryTimeRange += 1;
                    break;
                case TrainingResult::NotEnoughCollectedValues:
                    TS.TrainingResultNotEnoughCollectedValues += 1;
                    break;
                case TrainingResult::NullAcquiredDimension:
                    TS.TrainingResultNullAcquiredDimension += 1;
                    break;
                case TrainingResult::ChartUnderReplication:
                    TS.TrainingResultChartUnderReplication += 1;
                    break;
            }
        }

        std::this_thread::sleep_for(std::chrono::microseconds{RemainingUT});
    }
}

void Host::detect() {
    heartbeat_t HB;
    heartbeat_init(&HB);

    while (!netdata_exit) {
        heartbeat_next(&HB, RH->rrd_update_every * USEC_PER_SEC);
        detectOnce();
    }
}

void Host::getDetectionInfoAsJson(nlohmann::json &Json) const {
    Json["version"] = 1;
    Json["anomalous-dimensions"] = MLS.NumAnomalousDimensions;
    Json["normal-dimensions"] = MLS.NumNormalDimensions;
    Json["total-dimensions"] = MLS.NumAnomalousDimensions + MLS.NumNormalDimensions;
    Json["trained-dimensions"] = MLS.NumTrainingStatusTrained + MLS.NumTrainingStatusPendingWithModel;
}

void *train_main(void *Arg) {
    Host *H = reinterpret_cast<Host *>(Arg);
    H->train();
    return nullptr;
}

void *detect_main(void *Arg) {
    Host *H = reinterpret_cast<Host *>(Arg);
    H->detect();
    return nullptr;
}

void Host::startAnomalyDetectionThreads() {
    if (ThreadsRunning) {
        error("Anomaly detections threads for host %s are already-up and running.", rrdhost_hostname(RH));
        return;
    }

    ThreadsRunning = true;

    char Tag[NETDATA_THREAD_TAG_MAX + 1];

    snprintfz(Tag, NETDATA_THREAD_TAG_MAX, "TRAIN[%s]", rrdhost_hostname(RH));
    netdata_thread_create(&TrainingThread, Tag, NETDATA_THREAD_OPTION_JOINABLE, train_main, static_cast<void *>(this));

    snprintfz(Tag, NETDATA_THREAD_TAG_MAX, "DETECT[%s]", rrdhost_hostname(RH));
    netdata_thread_create(&DetectionThread, Tag, NETDATA_THREAD_OPTION_JOINABLE, detect_main, static_cast<void *>(this));
}

void Host::stopAnomalyDetectionThreads() {
    if (!ThreadsRunning) {
        error("Anomaly detections threads for host %s have already been stopped.", rrdhost_hostname(RH));
        return;
    }

    ThreadsRunning = false;

    // Signal the training queue to stop popping-items
    TrainingQueue.signal();
    netdata_thread_cancel(TrainingThread);
    netdata_thread_join(TrainingThread, nullptr);

    netdata_thread_cancel(DetectionThread);
    netdata_thread_join(DetectionThread, nullptr);
}
