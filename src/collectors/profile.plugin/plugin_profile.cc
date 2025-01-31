// SPDX-License-Identifier: GPL-3.0-or-later

#ifdef __cplusplus
extern "C" {
#endif

#include "database/rrd.h"

#ifdef __cplusplus
}
#endif

#include <random>
#include <thread>
#include <vector>

#define CONFIG_SECTION_PROFILE "plugin:profile"

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
    Profiler(size_t ID_arg, size_t NumCharts_arg, size_t NumDimsPerChart_arg, time_t SecondsToBackfill_arg, int UpdateEvery_arg) :
        ID(ID_arg),
        NumCharts(NumCharts_arg),
        NumDimsPerChart(NumDimsPerChart_arg),
        SecondsToBackfill(SecondsToBackfill_arg),
        UpdateEvery(UpdateEvery_arg),
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
                UpdateEvery, // update_every
                RRDSET_TYPE_LINE // chart_type
            );
            if (I != 0)
                rrdset_flag_set(RS, RRDSET_FLAG_HIDDEN);
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
        #define WORKER_JOB_CREATE_CHARTS 0
        #define WORKER_JOB_UPDATE_CHARTS 1
        #define WORKER_JOB_METRIC_DURATION_TO_BACKFILL 2
        #define WORKER_JOB_METRIC_POINTS_BACKFILLED 3

        worker_register("PROFILER");
        worker_register_job_name(WORKER_JOB_CREATE_CHARTS, "create charts");
        worker_register_job_name(WORKER_JOB_UPDATE_CHARTS, "update charts");
        worker_register_job_custom_metric(WORKER_JOB_METRIC_DURATION_TO_BACKFILL, "duration to backfill", "seconds", WORKER_METRIC_ABSOLUTE);
        worker_register_job_custom_metric(WORKER_JOB_METRIC_POINTS_BACKFILLED, "points backfilled", "points", WORKER_METRIC_ABSOLUTE);

        heartbeat_t HB;
        heartbeat_init(&HB, UpdateEvery * USEC_PER_SEC);

        worker_is_busy(WORKER_JOB_CREATE_CHARTS);
        create();

        struct timeval CollectionTV;
        now_realtime_timeval(&CollectionTV);

        if (SecondsToBackfill) {
            CollectionTV.tv_sec -= SecondsToBackfill;
            CollectionTV.tv_sec -= (CollectionTV.tv_sec % UpdateEvery);

            CollectionTV.tv_usec = 0;
        }

        size_t BackfilledPoints = 0;
        struct timeval NowTV, PrevTV;
        now_realtime_timeval(&NowTV);
        PrevTV = NowTV;

        while (service_running(SERVICE_COLLECTORS)) {
            worker_is_busy(WORKER_JOB_UPDATE_CHARTS);

            update(CollectionTV);
            CollectionTV.tv_sec += UpdateEvery;

            now_realtime_timeval(&NowTV);

            ++BackfilledPoints;
            if (NowTV.tv_sec > PrevTV.tv_sec) {
                PrevTV = NowTV;
                worker_set_metric(WORKER_JOB_METRIC_POINTS_BACKFILLED, BackfilledPoints * NumCharts * NumDimsPerChart);
                BackfilledPoints = 0;
            }

            size_t RemainingSeconds = (CollectionTV.tv_sec >= NowTV.tv_sec) ? 0 : (NowTV.tv_sec - CollectionTV.tv_sec);
            worker_set_metric(WORKER_JOB_METRIC_DURATION_TO_BACKFILL, RemainingSeconds);

            if (CollectionTV.tv_sec >= NowTV.tv_sec) {
                worker_is_idle();
                heartbeat_next(&HB);
            }
        }
    }

private:
    size_t ID;
    size_t NumCharts;
    size_t NumDimsPerChart;
    size_t SecondsToBackfill;
    int UpdateEvery;

    Generator Gen;
    std::vector<RRDSET *> Charts;
    std::vector<RRDDIM *> Dimensions;
};

static void *subprofile_main(void* Arg) {
    Profiler *P = reinterpret_cast<Profiler *>(Arg);
    P->run();
    return nullptr;
}

static void profile_main_cleanup(void *pptr) {
    struct netdata_static_thread *static_thread = (struct netdata_static_thread *)CLEANUP_FUNCTION_GET_PTR(pptr);
    if(!static_thread) return;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITING;

    static_thread->enabled = NETDATA_MAIN_THREAD_EXITED;
}

extern "C" void *profile_main(void *ptr) {
    CLEANUP_FUNCTION_REGISTER(profile_main_cleanup) cleanup_ptr = ptr;

    int UpdateEvery = (int) inicfg_get_duration_seconds(&netdata_config, CONFIG_SECTION_PROFILE, "update every", 1);
    if (UpdateEvery < localhost->rrd_update_every) {
        UpdateEvery = localhost->rrd_update_every;
        inicfg_set_duration_seconds(&netdata_config, CONFIG_SECTION_PROFILE, "update every", UpdateEvery);
    }

    // pick low-default values, in case this plugin is ever enabled accidentaly.
    size_t NumThreads = inicfg_get_number(&netdata_config, CONFIG_SECTION_PROFILE, "number of threads", 2);
    size_t NumCharts = inicfg_get_number(&netdata_config, CONFIG_SECTION_PROFILE, "number of charts", 2);
    size_t NumDimsPerChart = inicfg_get_number(&netdata_config, CONFIG_SECTION_PROFILE, "number of dimensions per chart", 2);
    size_t SecondsToBackfill = inicfg_get_number(&netdata_config, CONFIG_SECTION_PROFILE, "seconds to backfill", 10 * 60);

    std::vector<Profiler> Profilers;

    for (size_t Idx = 0; Idx != NumThreads; Idx++) {
        Profiler P(1e8 + Idx * 1e6, NumCharts, NumDimsPerChart, SecondsToBackfill, UpdateEvery);
        Profilers.push_back(P);
    }

    std::vector<ND_THREAD *> Threads(NumThreads);

    for (size_t Idx = 0; Idx != NumThreads; Idx++) {
        char Tag[NETDATA_THREAD_TAG_MAX + 1];

        snprintfz(Tag, NETDATA_THREAD_TAG_MAX, "PROFILER[%zu]", Idx);
        Threads[Idx] = nd_thread_create(Tag, NETDATA_THREAD_OPTION_JOINABLE,
                                        subprofile_main, static_cast<void *>(&Profilers[Idx]));
    }

    for (size_t Idx = 0; Idx != NumThreads; Idx++)
        nd_thread_join(Threads[Idx]);

    return NULL;
}
