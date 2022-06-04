// SPDX-License-Identifier: GPL-3.0-or-later

#include "plugin_profile.h"

#include <chrono>
#include <vector>
#include <random>
#include <sstream>
#include <string>
#include <map>

#define PLUGIN_PROFILE_NAME "profile.plugin"

#define CONFIG_SECTION_PROFILE "plugin:profile"

class Dimension {
public:
    Dimension(RRDDIM *RD, size_t Index) :
        RD(RD), Index(Index), NumUpdates(0) {}

    void update(size_t NumIterations) {
        if ((NumIterations % RD->update_every) != 0)
            return;

        collected_number CN = ++NumUpdates % 3 ? Index + 1 : Index;
        rrddim_set_by_pointer(RD->rrdset, RD, CN);
    }

private:
    RRDDIM *RD;
    size_t Index;
    size_t NumUpdates;
};

class Chart {
    static size_t ChartIdx;

public:
    static Chart *create(const char *Id, size_t UpdateEvery) {
        RRDSET *RS = rrdset_create(
            localhost,
            "prof_type",
            Id, // id
            NULL, // name
            "prof_family",
            NULL, // ctx
            Id, // title
            "prof_units",
            "prof_plugin",
            "prof_module",
            41000 + ChartIdx++,
            static_cast<int>(UpdateEvery),
            RRDSET_TYPE_LINE
        );
        if (!RS)
            fatal("Could not create chart %s", Id);

        return new Chart(RS);
    }

private:
    Chart(RRDSET *RS) : RS(RS) {}

public:
    void createDimension(const char *Name, size_t Index) {
        RRDDIM *RD = rrddim_add(
            RS, Name, NULL, 1, 1, RRD_ALGORITHM_ABSOLUTE
        );
        if (!RD)
            fatal("Could not create dimension %s.%s", RS->id, Name);

        Dimensions.push_back(new Dimension(RD, Index));
    }

    void update(size_t NumIterations) {
        if ((NumIterations % RS->update_every) != 0)
            return;

        rrdset_next(RS);
        for (Dimension *D : Dimensions)
            D->update(NumIterations);
        rrdset_done(RS);
    }

private:
    RRDSET *RS;
    std::vector<Dimension *> Dimensions;
};

size_t Chart::ChartIdx = 0;

static void profile_main_cleanup(void *ptr)
{
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

static std::vector<Chart *> createCharts(size_t NumCharts, unsigned NumDimsPerChart) {
    std::vector<Chart *> Charts;
    Charts.reserve(NumCharts);

    constexpr size_t Len = 1024;
    char Buf[Len];

    for (size_t ChartIdx = 0; ChartIdx != NumCharts; ChartIdx++) {
        snprintfz(Buf, Len, "chart_%zu", ChartIdx + 1);
        Chart *C = Chart::create(Buf, (ChartIdx % 2) + 1);

        for (size_t DimIdx = 0; DimIdx != NumDimsPerChart; DimIdx++) {
            snprintfz(Buf, Len, "dim_%zu", DimIdx + 1);
            C->createDimension(Buf, DimIdx + 1);
        }

        Charts.push_back(C);
    }

    return Charts;
}

static void updateCharts(const std::vector<Chart *> Charts, size_t NumIterations) {
    for (Chart *C : Charts)
        C->update(NumIterations);
}

void *profile_main(void *ptr)
{
    netdata_thread_cleanup_push(profile_main_cleanup, ptr);

    size_t NumCharts = 1;
    size_t NumDimsPerChart = 1;

    std::vector<Chart *> Charts = createCharts(NumCharts, NumDimsPerChart);

    heartbeat_t HB;
    heartbeat_init(&HB);
    size_t NumIterations = 0;

    while (!netdata_exit) {
        heartbeat_next(&HB, USEC_PER_SEC);
        updateCharts(Charts, ++NumIterations);
    }

    netdata_thread_cleanup_pop(1);
    return NULL;
}
