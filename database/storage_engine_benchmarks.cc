#include "daemon/common.h"
#include "storage_engine_benchmarks.h"

#include <atomic>
#include <cstddef>
#include <thread>
#include <unordered_map>
#include <vector>
#include <chrono>
#include <mutex>
#include <condition_variable>
#include <utility>
#include <string>

static STORAGE_ENGINE *se = nullptr;
static STORAGE_INSTANCE *si = nullptr;

typedef struct {
    STORAGE_METRICS_GROUP *smg;
    STORAGE_METRIC_HANDLE *smh;
    STORAGE_COLLECT_HANDLE *sch;
    RRDDIM rd;
} dimension_t;

class Barrier
{
public:
    Barrier(int count) : thread_count(count), counter(0), waiting(0) { }

    void wait() {
        std::unique_lock<std::mutex> lk(m);
        ++counter;
        ++waiting;
        cv.wait(lk, [&]{return counter >= thread_count;});
        cv.notify_one();

        --waiting;

        if(waiting == 0)
           counter = 0;

        lk.unlock();
    }

 private:
      std::mutex m;
      std::condition_variable cv;
      int thread_count;
      int counter;
      int waiting;
};

static std::size_t getRSS() {
    struct rusage usage;
    getrusage(RUSAGE_SELF, &usage);

    // return in MiB
    return usage.ru_maxrss / 1024;
}

static void gen_random_dimensions(std::vector<dimension_t> &dimensions,
                                  size_t num_groups,
                                  size_t num_dims_per_group)
{
    dimensions.reserve(num_groups * num_dims_per_group);

    for (size_t i = 0; i != num_groups; i++) {
        uuid_t smg_uuid;
        uuid_generate(smg_uuid);

        STORAGE_METRICS_GROUP *smg = storage_engine_metrics_group_get(STORAGE_ENGINE_BACKEND_DBENGINE, si, &smg_uuid);

        for (size_t j = 0; j != num_dims_per_group; j++) {
            dimension_t d;

            uuid_generate(d.rd.metric_uuid);
            d.smh = se->api.metric_get_or_create(&d.rd, si);
            d.sch = storage_metric_store_init(STORAGE_ENGINE_BACKEND_DBENGINE, d.smh, 1, smg);
            d.smg = smg;

            dimensions.push_back(d);
        }
    }
}

static void gen_random_data(std::vector<dimension_t> &dimensions, size_t num_points_per_dimension)
{
    usec_t point_in_time = (now_realtime_sec() - (365 * 24 * 3600)) * USEC_PER_SEC;

    for (size_t i = 0; i != num_points_per_dimension; i++) {
        for (size_t j = 0; j != dimensions.size(); j++) {
            storage_engine_store_metric(dimensions[j].sch, point_in_time, i, 0, 0, 1, 0, SN_DEFAULT_FLAGS);
        }
        point_in_time += USEC_PER_SEC;
    }

    for (size_t i = 0; i != dimensions.size(); i++)
        storage_engine_store_flush(dimensions[i].sch);
}

Barrier *B = nullptr;

static void gen_thread(size_t thread_id, size_t num_groups, size_t num_dims_per_group, size_t num_points_per_dimension) {
    char thread_name[128];
    snprintfz(thread_name, 1024, "genthread-%04zu", thread_id);
    pthread_setname_np(pthread_self(), thread_name);

    std::vector<dimension_t> dimensions;
    gen_random_dimensions(dimensions, num_groups, num_dims_per_group);

    B->wait();

    gen_random_data(dimensions, num_points_per_dimension);

    netdata_log_error("Thread %zu finished...", thread_id);
}

static size_t numFlushedPages() {
    return  __atomic_load_n(&multidb_ctx[0]->atomic.num_flushed_pages, __ATOMIC_ACQUIRE);
}

static std::unordered_map<std::string, size_t> parseOptions(int argc, char *argv[])
{
    // Default values make each thread represent the amount of work done
    // by a regular child agent connected to a parent, ie. you can simulate
    // X children by setting the number of threads to X.
    std::unordered_map<std::string, size_t> Opts =
    {
        {"--num-threads", get_netdata_cpus()},
        {"--num-groups", 500},
        {"--num-dimensions-per-group", 5},
        {"--num-points-per-dimension", 7 * 24 * 3600},
        {"--num-seconds-to-run", 60},
    };

    for (int Idx = 3; Idx < argc; ++Idx)
    {
        std::string Arg = argv[Idx];

        if (Idx < argc)
        {
            size_t Value = std::stoi(argv[Idx + 1]);
            // If the argument is a known option, store the value
            if (Opts.find(Arg) != Opts.end()) {
                Opts[Arg] = Value;
                Idx++; // Increment the counter to skip the next argument, since it's a value
            }
        }
    }

    return Opts;
}

static bool initializeDBEngine()
{
    std::string path = std::string(netdata_configured_cache_dir) + "/se-benchmarks";
    std::string command = "rm -rf " + path + " && mkdir -p " + path;
    if (std::system(command.c_str()) != 0)
        return false;

    auto start_time = std::chrono::high_resolution_clock::now();

    dbengine_init((char *) "dummy-hostname", path.c_str());

    auto end_time = std::chrono::high_resolution_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(end_time - start_time);
    double seconds = duration.count() / static_cast<double>(MSEC_PER_SEC);
    netdata_log_error("DB-engine initialization time: %.2lf seconds", seconds);
    return true;
}

int storage_engine_benchmarks(int argc, char *argv[])
{
    std::unordered_map<std::string, size_t> Options = parseOptions(argc, argv);

    size_t num_threads = Options["--num-threads"];
    size_t num_groups = Options["--num-groups"];
    size_t num_dims_per_group = Options["--num-dimensions-per-group"];
    size_t num_points_per_dimension = Options["--num-points-per-dimension"];
    size_t num_seconds_to_run = Options["--num-seconds-to-run"];

    error_log_limit_unlimited();

    netdata_log_error("Test configuration: threads=%zu, groups=%zu, dims_per_group=%zu, points_per_dim=%zu",
                      num_threads, num_groups, num_dims_per_group, num_points_per_dimension);

    if (!initializeDBEngine())
        return 1;

    se = storage_engine_get(RRD_MEMORY_MODE_DBENGINE);
    si = reinterpret_cast<STORAGE_INSTANCE *>(multidb_ctx[0]);

    Barrier Bar(num_threads + 1);
    B = &Bar;

    std::vector<std::thread> threads;
    {
        auto start_time = std::chrono::high_resolution_clock::now();

        for (size_t i = 0; i < num_threads; ++i)
            threads.emplace_back(gen_thread, i, num_groups, num_dims_per_group, num_points_per_dimension);

        B->wait();

        auto end_time = std::chrono::high_resolution_clock::now();
        auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(end_time - start_time);
        double seconds = duration.count() / static_cast<double>(MSEC_PER_SEC);
        netdata_log_error("Time to setup metrics: %.2lf seconds (RSS: %zu MiB)", seconds, getRSS());
    }

    {
        while (num_seconds_to_run-- > 0)
        {
            auto start_time = std::chrono::high_resolution_clock::now();
            size_t PrevNumFlushedPages = numFlushedPages();
            std::this_thread::sleep_for(std::chrono::seconds{1});
            size_t CurrNumFlushedPages = numFlushedPages();
            auto end_time = std::chrono::high_resolution_clock::now();

            auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(end_time - start_time);
            double seconds = duration.count() / static_cast<double>(MSEC_PER_SEC);

            double pages_per_second = static_cast<double>(CurrNumFlushedPages - PrevNumFlushedPages) / seconds;
            double points_per_sec = pages_per_second * 1024;
            double mib_per_sec = (points_per_sec * sizeof(storage_number)) / (1024.0 * 1024.0);

            double capacity = points_per_sec / 2500.0;

            netdata_log_error("pages/sec: %.2lf, points/sec: %.2lf, mib/sec: %.2lf, capacity: %.2lf (RSS: %zu MiB)",
                              pages_per_second, points_per_sec, mib_per_sec, capacity, getRSS());
        }
    }

    netdata_log_error("Storage engine benchmark finished. Joining threads...");
    for (std::thread& thread : threads)
        thread.join();

    exit(EXIT_SUCCESS);
}
