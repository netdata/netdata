// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Host.h"

using namespace ml;

void ml_init(void) {
    Cfg.readMLConfig();
}

/*
 * Assumptions:
 *  1) hosts outlive their sets, and sets outlive their dimensions,
 *  2) dimensions always have a set that has a host.
 */
void ml_new_host(RRDHOST *RH) {
    if (simple_pattern_matches(Cfg.SP_HostsToSkip, RH->hostname))
        return;

    Host *H = new Host(RH);
    RH->ml_host = static_cast<ml_host_t>(H);
}

void ml_delete_host(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->ml_host);
    if (!H)
        return;

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
