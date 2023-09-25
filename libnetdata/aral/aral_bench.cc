#include "libnetdata/libnetdata.h"

#include <benchmark/benchmark.h>
#include <iostream>
#include <string>

static std::vector<ARAL *> arals;

static ARAL *get_aral_for_thread(size_t index) {
    return arals[index % arals.size()];
}

static void BM_aral(benchmark::State& state) {
    ARAL *aral = get_aral_for_thread(state.thread_index());

    for (auto _ : state) {
        void *buf = aral_mallocz(aral);
        memset(buf, 0, GORILLA_BUFFER_SIZE);
        aral_freez(aral, buf);
    }

    state.SetItemsProcessed(state.iterations());
    state.SetBytesProcessed(state.iterations() * GORILLA_BUFFER_SIZE);
}
BENCHMARK(BM_aral)->ThreadRange(1, 512)->UseRealTime();

extern "C" int aral_benchmark(int argc, char *argv[])
{
    const size_t num_arals = 24;

    arals.reserve(num_arals);

    for (size_t i = 0; i != num_arals; i++)
    {
        char buf[20 + 1];
        snprintfz(buf, 20, "aral-%zu", i);

        ARAL *aral = aral_create(
                buf,
                GORILLA_BUFFER_SIZE,
                64,
                512 * GORILLA_BUFFER_SIZE,
                NULL,
                NULL, NULL, false, false);
        arals.push_back(aral);
    }

   ::benchmark::Initialize(&argc, argv);
   ::benchmark::RunSpecifiedBenchmarks();

    return 0;
}
