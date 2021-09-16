// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Host.h"

using namespace ml;

/*
 * Assumptions:
 *  1) hosts outlive their sets, and sets outlive their dimensions,
 *  2) dimensions always have a set that has a host.
 */

void ml_init(void) {
    Cfg.readMLConfig();
}

void ml_new_host(RRDHOST *RH) {
    if (simple_pattern_matches(Cfg.SP_HostsToSkip, RH->hostname))
        return;

    Host *H = new Host(RH);
    RH->ml_host = static_cast<ml_host_t>(H);

    H->startTrainingThread();
}

void ml_delete_host(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->ml_host);
    if (!H)
        return;

    H->stopTrainingThread();

    delete H;
    RH->ml_host = nullptr;
}

void ml_new_dimension(RRDDIM *RD) {
    RRDSET *RS = RD->rrdset;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    if (!H)
        return;

    if (simple_pattern_matches(Cfg.SP_ChartsToSkip, RS->name))
        return;

    Dimension *D = new Dimension(RD);
    RD->state->ml_dimension = static_cast<ml_dimension_t>(D);
    H->addDimension(D);
}

void ml_delete_dimension(RRDDIM *RD) {
    Dimension *D = static_cast<Dimension *>(RD->state->ml_dimension);
    if (!D)
        return;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    H->removeDimension(D);

    delete D;
    RD->state->ml_dimension = nullptr;
}

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    Dimension *D = static_cast<Dimension *>(RD->state->ml_dimension);
    if (!D)
        return false;

    D->addValue(Value, Exists);
    bool Result = D->predict().second;
    error("Dimension %s anomaly bit %d", RD->name, Result);
    return Result;
}

#if defined(ENABLE_ML_TESTS)

#include "gtest/gtest.h"

int test_ml(int argc, char *argv[]) {
    (void) argc;
    (void) argv;

    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

#endif // ENABLE_ML_TESTS
