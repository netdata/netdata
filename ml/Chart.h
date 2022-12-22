// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_CHART_H
#define ML_CHART_H

#include "Config.h"
#include "Dimension.h"

#include "ml-private.h"
#include "json/single_include/nlohmann/json.hpp"

namespace ml
{

class Chart {
public:
    Chart(RRDSET *RS) :
        RS(RS),
        MLS()
     { }

    RRDSET *getRS() const {
        return RS;
    }

    bool isAvailableForML() {
        return rrdset_is_available_for_exporting_and_alarms(RS);
    }

    void addDimension(Dimension *D) {
        std::lock_guard<Mutex> L(M);
        Dimensions[D->getRD()] = D;
    }

    void removeDimension(Dimension *D) {
        std::lock_guard<Mutex> L(M);
        Dimensions.erase(D->getRD());
    }

    void getModelsAsJson(nlohmann::json &Json) {
        std::lock_guard<Mutex> L(M);

        for (auto &DP : Dimensions) {
            Dimension *D = DP.second;
            nlohmann::json JsonArray = nlohmann::json::array();
            for (const KMeans &KM : D->getModels()) {
                nlohmann::json J;
                KM.toJson(J);
                JsonArray.push_back(J);
            }

            Json[getMLDimensionID(D->getRD())] = JsonArray;
        }
    }

    void updateBegin() {
        M.lock();
        MLS = {};
    }

    void updateDimension(Dimension *D, bool IsAnomalous) {
        switch (D->getMLS()) {
            case MachineLearningStatus::DisabledDueToUniqueUpdateEvery:
                MLS.NumMachineLearningStatusDisabledUE++;
                return;
            case MachineLearningStatus::DisabledDueToExcludedChart:
                MLS.NumMachineLearningStatusDisabledSP++;
                return;
            case MachineLearningStatus::Enabled: {
                MLS.NumMachineLearningStatusEnabled++;

                switch (D->getMT()) {
                    case MetricType::Constant:
                        MLS.NumMetricTypeConstant++;
                        MLS.NumTrainingStatusTrained++;
                        MLS.NumNormalDimensions++;
                        return;
                    case MetricType::Variable:
                        MLS.NumMetricTypeVariable++;
                        break;
                }

                switch (D->getTS()) {
                    case TrainingStatus::Untrained:
                        MLS.NumTrainingStatusUntrained++;
                        return;
                    case TrainingStatus::PendingWithoutModel:
                        MLS.NumTrainingStatusPendingWithoutModel++;
                        return;
                    case TrainingStatus::Trained:
                        MLS.NumTrainingStatusTrained++;

                        MLS.NumAnomalousDimensions += IsAnomalous;
                        MLS.NumNormalDimensions += !IsAnomalous;
                        return;
                    case TrainingStatus::PendingWithModel:
                        MLS.NumTrainingStatusPendingWithModel++;

                        MLS.NumAnomalousDimensions += IsAnomalous;
                        MLS.NumNormalDimensions += !IsAnomalous;
                        return;
                }

                return;
            }
        }
    }

    void updateEnd() {
        M.unlock();
    }

    MachineLearningStats getMLS() {
        std::lock_guard<Mutex> L(M);
        return MLS;
    }

private:
    RRDSET *RS;
    MachineLearningStats MLS;

    Mutex M;
    std::unordered_map<RRDDIM *, Dimension *> Dimensions;
};

} // namespace ml

#endif /* ML_CHART_H */
