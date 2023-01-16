// SPDX-License-Identifier: GPL-3.0-or-later

#ifdef __cplusplus
extern "C" {
#endif

#include "daemon/common.h"
#include "libnetdata/os.h"

#ifdef __cplusplus
};
#endif

#include <random>
#include <thread>
#include <vector>

#define PLUGIN_TIMEX_NAME "profile.plugin"

#define CONFIG_SECTION_TIMEX "plugin:profile"

class Generator {
public:
    Generator(size_t N) : Offset(0) {
        std::random_device RandDev;
        std::mt19937 Gen(RandDev());
        std::uniform_int_distribution<int> D(-16, 16);

        V.reserve(N);
        for (size_t Idx = 0; Idx != N; Idx++)
            V.push_back(D(Gen));
    }

    double getRandValue() {
        return V[Offset++ % V.size()];
    }

private:
    size_t Offset;
    std::vector<double> V;
};

class Profiler {
public:
    Profiler(size_t ID, size_t NumCharts, size_t NumDimsPerChart, time_t SecondsToBackfill) :
        ID(ID),
        NumCharts(NumCharts),
        NumDimsPerChart(NumDimsPerChart),
        SecondsToBackfill(SecondsToBackfill),
        Gen(1024 * 1024)
    {}

    void create() {
        char ChartId[1024];
        char DimId[1024];

        Charts.reserve(NumCharts);
        for (size_t I = 0; I != NumCharts; I++) {
            size_t CID = ID + Charts.size() + 1;

            snprintfz(ChartId, 1024 - 1, "chart_%zu", CID);

            RRDSET *RS = rrdset_create_localhost(
                "profile", // type
                ChartId, // id
                nullptr, // name,
                "profile_family", // family
                "profile_context", // context
                "profile_title", // title
                "profile_units", // units
                "profile_plugin", // plugin
                "profile_module", // module
                12345678 + CID, // priority
                localhost->rrd_update_every, // update_every
                RRDSET_TYPE_LINE // chart_type
            );
            Charts.push_back(RS);

            Dimensions.reserve(NumDimsPerChart);
            for (size_t J = 0; J != NumDimsPerChart; J++) {
                snprintfz(DimId, 1024 - 1, "dim_%zu", J);

                RRDDIM *RD = rrddim_add(
                    RS, // st
                    DimId, // id
                    nullptr, // name
                    1, // multiplier
                    1, // divisor
                    RRD_ALGORITHM_ABSOLUTE // algorithm
                );

                Dimensions.push_back(RD);
            }
        }
    }

    void update(const struct timeval &Now) {
        for (RRDSET *RS: Charts) {
            for (RRDDIM *RD : Dimensions) {
                rrddim_timed_set_by_pointer(RS, RD, Now, Gen.getRandValue());
            }

            rrdset_timed_done(RS, Now, RS->counter_done != 0);
        }
    }

    void run() {
        heartbeat_t HB;
        heartbeat_init(&HB);

        create();

        struct timeval CollectionTV;
        now_realtime_timeval(&CollectionTV);

        if (SecondsToBackfill) {
            CollectionTV.tv_sec -= SecondsToBackfill;
            CollectionTV.tv_usec = 0;
        }

        while (service_running(SERVICE_COLLECTORS)) {
            update(CollectionTV);
            CollectionTV.tv_sec += 1;

            struct timeval NowTV;
            now_realtime_timeval(&NowTV);

            if (CollectionTV.tv_sec >= NowTV.tv_sec)
                heartbeat_next(&HB, USEC_PER_SEC);
        }
    }

private:
    size_t ID;
    size_t NumCharts;
    size_t NumDimsPerChart;
    size_t SecondsToBackfill;

    Generator Gen;
    std::vector<RRDSET *> Charts;
    std::vector<RRDDIM *> Dimensions;
};

void *subprofile_main(void* Arg) {
    Profiler *P = reinterpret_cast<Profiler *>(Arg);
    P->run();
    return nullptr;
}

static void profile_main_cleanup(void *ptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *) ptr;
    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    info("cleaning up...");

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

extern "C" void *profile_main(void *ptr) {
    netdata_thread_cleanup_push(profile_main_cleanup, ptr);

    std::vector<Profiler> Profilers;

    size_t NumThreads = 1;
    for (size_t Idx = 0; Idx != NumThreads; Idx++) {
        size_t NumCharts = 1;
        size_t NumDimsPerChart = 1;
        time_t SecondsToBackfill = 0;

        Profiler P(1e8 + Idx * 1e6, NumCharts, NumDimsPerChart, SecondsToBackfill);
        Profilers.push_back(P);
    }

    std::vector<netdata_thread_t> Threads(NumThreads);

    for (size_t Idx = 0; Idx != NumThreads; Idx++) {
        char Tag[NETDATA_THREAD_TAG_MAX + 1];

        snprintfz(Tag, NETDATA_THREAD_TAG_MAX, "PROFILE[%zu]", Idx);
        netdata_thread_create(&Threads[Idx], Tag, NETDATA_THREAD_OPTION_JOINABLE, subprofile_main, static_cast<void *>(&Profilers[Idx]));
    }

    for (size_t Idx = 0; Idx != NumThreads; Idx++)
        netdata_thread_join(Threads[Idx], nullptr);

    netdata_thread_cleanup_pop(1);
    return NULL;
}
