// SPDX-License-Identifier: GPL-3.0-or-later

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// platform readers - called only by the bootstrap read and the sampler thread,
// never concurrently, so the readers may keep private state

// Windows
#if defined(OS_WINDOWS)
#include <windows.h>

static OS_SYSTEM_MEMORY system_memory_read(bool query_total_ram __maybe_unused) {
    OS_SYSTEM_MEMORY sm = OS_SYSTEM_MEMORY_EMPTY;

    MEMORYSTATUSEX statex;
    statex.dwLength = sizeof(statex);
    if (GlobalMemoryStatusEx(&statex)) {
        sm.ram_total_bytes = statex.ullTotalPhys;
        sm.ram_available_bytes = statex.ullAvailPhys;
    }

    return sm;
}
#endif

// Linux
#if defined(OS_LINUX)

static OS_SYSTEM_MEMORY os_system_memory_cgroup_v1(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = OS_SYSTEM_MEMORY_EMPTY;
    char buf[4096];
    uint64_t used = 0, inactive = 0;

    if(query_total_ram || sm.ram_total_bytes == 0) {
        if (read_txt_file("/sys/fs/cgroup/memory/memory.limit_in_bytes", buf, sizeof(buf)) != 0)
            goto failed;

        sm.ram_total_bytes = str2ull(buf, NULL);
        if(!sm.ram_total_bytes)
            goto failed;
    }

    buf[0] = '\0';
    if (read_txt_file("/sys/fs/cgroup/memory/memory.usage_in_bytes", buf, sizeof(buf)) != 0)
        goto failed;

    used = str2ull(buf, NULL);
    if(!used || used > sm.ram_total_bytes)
        goto failed;

    if (read_txt_file("/sys/fs/cgroup/memory/memory.stat", buf, sizeof(buf)) != 0)
        goto done;

    const char *inactive_str = strstr(buf, "total_inactive_file ");
    if(!inactive_str)
        goto done;

    inactive_str += 20;

    inactive = str2ull(inactive_str, NULL);
    if(!inactive || inactive > used) {
        inactive = 0;
        goto done;
    }

done:
    sm.ram_available_bytes = sm.ram_total_bytes - (used - inactive);
    return sm;

failed:
    sm = OS_SYSTEM_MEMORY_EMPTY;
    return sm;
}

static OS_SYSTEM_MEMORY os_system_memory_cgroup_v2(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = OS_SYSTEM_MEMORY_EMPTY;
    char buf[4096];
    uint64_t used = 0, inactive = 0;

    if(query_total_ram || sm.ram_total_bytes == 0) {
        if (read_txt_file("/sys/fs/cgroup/memory.max", buf, sizeof(buf)) != 0)
            goto failed;

        if(strcmp(buf, "max") == 0)
            sm.ram_total_bytes = UINT64_MAX;
        else
            sm.ram_total_bytes = str2ull(buf, NULL);

        if(!sm.ram_total_bytes)
            goto failed;
    }

    buf[0] = '\0';
    if (read_txt_file("/sys/fs/cgroup/memory.current", buf, sizeof(buf)) != 0)
        goto failed;

    used = str2ull(buf, NULL);
    if(!used || used > sm.ram_total_bytes)
        goto failed;

    if (read_txt_file("/sys/fs/cgroup/memory.stat", buf, sizeof(buf)) != 0)
        goto done;

    const char *inactive_str = strstr(buf, "inactive_file ");
    if(!inactive_str)
        goto done;

    inactive_str += 14;

    inactive = str2ull(inactive_str, NULL);
    if(!inactive || inactive > used) {
        inactive = 0;
        goto done;
    }

done:
    sm.ram_available_bytes = sm.ram_total_bytes - (used - inactive);
    return sm;

failed:
    sm = OS_SYSTEM_MEMORY_EMPTY;
    return sm;
}

#define MEMINFO_MEMTOTAL "MemTotal:"
#define MEMINFO_MEMAVAILABLE "MemAvailable:"

static OS_SYSTEM_MEMORY os_system_memory_meminfo(bool query_total_ram __maybe_unused) {
    static OS_SYSTEM_MEMORY sm = OS_SYSTEM_MEMORY_EMPTY;

    char buf[4096];
    if (read_txt_file("/proc/meminfo", buf, sizeof(buf)) != 0)
        goto failed;

    char *s = strstr(buf, MEMINFO_MEMTOTAL);
    if(!s) goto failed;
    s += sizeof(MEMINFO_MEMTOTAL) - 1;
    while(isspace((uint8_t)*s)) s++;
    sm.ram_total_bytes = str2ull(s, NULL) * 1024;

    s = strstr(buf, MEMINFO_MEMAVAILABLE);
    if(!s) goto failed;
    s += sizeof(MEMINFO_MEMAVAILABLE) - 1;
    while(isspace((uint8_t)*s)) s++;
    sm.ram_available_bytes = str2ull(s, NULL) * 1024;

    return sm;

failed:
    sm.ram_total_bytes = 0;
    sm.ram_available_bytes = 0;
    return sm;
}

typedef enum {
    OS_MEM_SRC_UNKNOWN,
    OS_MEM_SRC_CGROUP_V1,
    OS_MEM_SRC_CGROUP_V2,
    OS_MEM_SRC_MEMINFO,
} OS_MEM_SRC;

// query_total_ram also re-detects the source (meminfo, cgroup v1 or v2)
static OS_SYSTEM_MEMORY system_memory_read(bool query_total_ram) {
    static OS_MEM_SRC src = OS_MEM_SRC_UNKNOWN;
    OS_SYSTEM_MEMORY sm;

    if(query_total_ram)
        // let it auto-detect
        src = OS_MEM_SRC_UNKNOWN;

    switch(src) {
        case OS_MEM_SRC_MEMINFO:
            sm = os_system_memory_meminfo(query_total_ram);
            break;

        case OS_MEM_SRC_CGROUP_V2:
            sm = os_system_memory_cgroup_v2(query_total_ram);
            break;

        case OS_MEM_SRC_CGROUP_V1:
            sm = os_system_memory_cgroup_v1(query_total_ram);
            break;

        default:
        case OS_MEM_SRC_UNKNOWN: {
            OS_SYSTEM_MEMORY mi = os_system_memory_meminfo(true);
            OS_SYSTEM_MEMORY v1 = os_system_memory_cgroup_v1(true);
            OS_SYSTEM_MEMORY v2 = os_system_memory_cgroup_v2(true);

            if(v2.ram_total_bytes && v2.ram_available_bytes && v2.ram_total_bytes <= mi.ram_total_bytes && v2.ram_available_bytes < mi.ram_available_bytes) {
                sm = v2;
                src = OS_MEM_SRC_CGROUP_V2;
            }
            else {
//                if(v2.ram_total_bytes || v2.ram_available_bytes)
//                    nd_log(NDLS_DAEMON, NDLP_ERR, "SYSTEM_MEMORY: cgroup v2 reports more memory than meminfo. Ignoring cgroup v2.");

                if (v1.ram_total_bytes && v1.ram_available_bytes && v1.ram_total_bytes <= mi.ram_total_bytes &&
                    v1.ram_available_bytes < mi.ram_available_bytes) {
                    sm = v1;
                    src = OS_MEM_SRC_CGROUP_V1;
                }
                else {
//                    if(v1.ram_total_bytes || v1.ram_available_bytes)
//                        nd_log(NDLS_DAEMON, NDLP_ERR, "SYSTEM_MEMORY: cgroup v1 reports more memory than meminfo. Ignoring cgroup v1.");

                    sm = mi;
                    src = OS_MEM_SRC_MEMINFO;
                }
            }
        }
    }

    if(!OS_SYSTEM_MEMORY_OK(sm))
        // the source failed - detect it again on the next read
        src = OS_MEM_SRC_UNKNOWN;

    return sm;
}
#endif

// FreeBSD
#if defined(OS_FREEBSD)
#include <sys/types.h>
#include <sys/sysctl.h>

static OS_SYSTEM_MEMORY system_memory_read(bool query_total_ram) {
    static OS_SYSTEM_MEMORY sm = OS_SYSTEM_MEMORY_EMPTY;

    // Query the total RAM only if needed or if it hasn't been cached
    if (query_total_ram || sm.ram_total_bytes == 0) {
        uint64_t total_pages = 0;
        size_t size = sizeof(total_pages);
        if (sysctlbyname("vm.stats.vm.v_page_count", &total_pages, &size, NULL, 0) != 0)
            goto failed;

        unsigned long page_size = 0;
        size = sizeof(page_size);
        if (sysctlbyname("hw.pagesize", &page_size, &size, NULL, 0) != 0)
            goto failed;

        sm.ram_total_bytes = total_pages * page_size;
    }

    // Query the available RAM (free + inactive pages)
    uint64_t free_pages = 0, inactive_pages = 0;
    size_t size = sizeof(free_pages);
    if (sysctlbyname("vm.stats.vm.v_free_count", &free_pages, &size, NULL, 0) != 0 ||
        sysctlbyname("vm.stats.vm.v_inactive_count", &inactive_pages, &size, NULL, 0) != 0)
        goto failed;

    unsigned long page_size = 0;
    size = sizeof(page_size);
    if (sysctlbyname("hw.pagesize", &page_size, &size, NULL, 0) != 0)
        goto failed;

    sm.ram_available_bytes = (free_pages + inactive_pages) * page_size;

    return sm;

failed:
    sm = OS_SYSTEM_MEMORY_EMPTY;
    return sm;
}
#endif

// macOS
#if defined(OS_MACOS)
#include <mach/mach.h>
#include <sys/sysctl.h>

static OS_SYSTEM_MEMORY system_memory_read(bool query_total_ram) {
    static uint64_t total_ram = 0;
    static uint64_t page_size = 0;

    if (page_size == 0) {
        size_t len = sizeof(page_size);
        if (sysctlbyname("hw.pagesize", &page_size, &len, NULL, 0) != 0)
            return OS_SYSTEM_MEMORY_EMPTY;
    }

    if (query_total_ram || total_ram == 0) {
        size_t len = sizeof(total_ram);
        if (sysctlbyname("hw.memsize", &total_ram, &len, NULL, 0) != 0)
            return OS_SYSTEM_MEMORY_EMPTY;
    }

    uint64_t ram_available = 0;
    if (page_size > 0) {
        vm_statistics64_data_t vm_info;
        mach_msg_type_number_t count = HOST_VM_INFO64_COUNT;
        mach_port_t mach_port = mach_host_self();

        if (host_statistics64(mach_port, HOST_VM_INFO64, (host_info_t)&vm_info, &count) != KERN_SUCCESS) {
            mach_port_deallocate(mach_task_self(), mach_port);
            return OS_SYSTEM_MEMORY_EMPTY;
        }

        ram_available = (vm_info.free_count + vm_info.inactive_count + vm_info.purgeable_count) * page_size;
        mach_port_deallocate(mach_task_self(), mach_port);
    }

    return (OS_SYSTEM_MEMORY){
        .ram_total_bytes = total_ram,
        .ram_available_bytes = ram_available,
    };
}
#endif

// ----------------------------------------------------------------------------
// the published sample

#define SYSTEM_MEMORY_SAMPLE_EVERY_MS 1000
#define SYSTEM_MEMORY_REDETECT_EVERY_UT (60 * USEC_PER_SEC)

// A seqlock with a single writer (the bootstrap read, then the sampler - never both at once):
// seq is odd while a sample is being written, so a reader never mixes the fields of two samples.
static struct {
    uint32_t seq;
    uint64_t ram_total_bytes;
    uint64_t ram_available_bytes;
    usec_t sampled_ut;              // monotonic time of the last valid sample, 0 = none yet
} published = { 0 };

// only valid samples are published: a failed read keeps the previous sample, and lets it age
static void system_memory_publish(OS_SYSTEM_MEMORY sm) {
    if(!OS_SYSTEM_MEMORY_OK(sm))
        return;

    usec_t now_ut = now_monotonic_usec();   // outside the write window, to keep it short

    uint32_t seq = __atomic_load_n(&published.seq, __ATOMIC_RELAXED);
    __atomic_store_n(&published.seq, seq + 1, __ATOMIC_RELAXED);
    __atomic_thread_fence(__ATOMIC_RELEASE);

    __atomic_store_n(&published.ram_total_bytes, sm.ram_total_bytes, __ATOMIC_RELAXED);
    __atomic_store_n(&published.ram_available_bytes, sm.ram_available_bytes, __ATOMIC_RELAXED);
    __atomic_store_n(&published.sampled_ut, now_ut, __ATOMIC_RELAXED);

    __atomic_store_n(&published.seq, seq + 2, __ATOMIC_RELEASE);
}

OS_SYSTEM_MEMORY os_system_memory_cached(usec_t max_age_ut) {
    OS_SYSTEM_MEMORY sm;
    usec_t sampled_ut;

    // the writer does three stores and no I/O, so a retry is rare; still bounded, since this runs on fatal paths
    for(size_t tries = 0; ; tries++) {
        if(tries >= 1000)
            return OS_SYSTEM_MEMORY_EMPTY;

        uint32_t seq = __atomic_load_n(&published.seq, __ATOMIC_ACQUIRE);
        if(seq & 1) {
            yield_the_processor();
            continue;
        }

        sm.ram_total_bytes = __atomic_load_n(&published.ram_total_bytes, __ATOMIC_RELAXED);
        sm.ram_available_bytes = __atomic_load_n(&published.ram_available_bytes, __ATOMIC_RELAXED);
        sampled_ut = __atomic_load_n(&published.sampled_ut, __ATOMIC_RELAXED);

        __atomic_thread_fence(__ATOMIC_ACQUIRE);
        if(__atomic_load_n(&published.seq, __ATOMIC_RELAXED) == seq)
            break;
    }

    if(!sampled_ut)
        return OS_SYSTEM_MEMORY_EMPTY;

    if(max_age_ut && now_monotonic_usec() > sampled_ut + max_age_ut)
        return OS_SYSTEM_MEMORY_EMPTY;

    return sm;
}

// ----------------------------------------------------------------------------
// the first read, before the sampler runs

typedef enum {
    SYSTEM_MEMORY_BOOTSTRAP_NONE = 0,
    SYSTEM_MEMORY_BOOTSTRAP_RUNNING,
    SYSTEM_MEMORY_BOOTSTRAP_DONE,
} SYSTEM_MEMORY_BOOTSTRAP;

static SYSTEM_MEMORY_BOOTSTRAP bootstrap_state = SYSTEM_MEMORY_BOOTSTRAP_NONE;

// One read, once per process, even if it fails: after it, only the sampler reads.
// Concurrent callers sleep (not spin) until it finishes, which may take as long as the read blocks.
static void system_memory_bootstrap(void) {
    if(__atomic_load_n(&bootstrap_state, __ATOMIC_ACQUIRE) == SYSTEM_MEMORY_BOOTSTRAP_DONE)
        return;

    SYSTEM_MEMORY_BOOTSTRAP expected = SYSTEM_MEMORY_BOOTSTRAP_NONE;
    if(__atomic_compare_exchange_n(&bootstrap_state, &expected, SYSTEM_MEMORY_BOOTSTRAP_RUNNING,
                                   false, __ATOMIC_ACQ_REL, __ATOMIC_ACQUIRE)) {
        system_memory_publish(system_memory_read(true));
        __atomic_store_n(&bootstrap_state, SYSTEM_MEMORY_BOOTSTRAP_DONE, __ATOMIC_RELEASE);
        return;
    }

    while(__atomic_load_n(&bootstrap_state, __ATOMIC_ACQUIRE) != SYSTEM_MEMORY_BOOTSTRAP_DONE)
        sleep_usec(USEC_PER_MS);
}

OS_SYSTEM_MEMORY os_system_memory(void) {
    system_memory_bootstrap();
    return os_system_memory_cached(0);
}

// ----------------------------------------------------------------------------
// the sampler thread

static struct {
    ND_THREAD *thread;
    struct completion wake;         // shortens the sleep when the thread is asked to stop
    bool exited;
    bool join_failed;               // the handle was given up without a clean join
} sampler = { 0 };

static void system_memory_sampler_cancel(void *data __maybe_unused) {
    completion_mark_complete_a_job(&sampler.wake);
}

static void system_memory_sampler_thread(void *ptr __maybe_unused) {
    worker_register("SYSMEM");
    worker_register_job_name(0, "read");
    nd_thread_register_canceller(system_memory_sampler_cancel, NULL);

    usec_t last_redetect_ut = now_monotonic_usec();
    unsigned job_id = 0;

    while(!nd_thread_signaled_to_cancel()) {
        worker_is_idle();
        job_id = completion_wait_for_a_job_with_timeout(&sampler.wake, job_id, SYSTEM_MEMORY_SAMPLE_EVERY_MS);

        if(nd_thread_signaled_to_cancel())
            break;

        worker_is_busy(0);

        usec_t now_ut = now_monotonic_usec();
        bool redetect = now_ut - last_redetect_ut >= SYSTEM_MEMORY_REDETECT_EVERY_UT;
        if(redetect)
            last_redetect_ut = now_ut;

        system_memory_publish(system_memory_read(redetect));
    }

    worker_unregister();
    __atomic_store_n(&sampler.exited, true, __ATOMIC_RELEASE);
}

void os_system_memory_sampler_start(void) {
    FUNCTION_RUN_ONCE();

    // the sampler must be the only reader: finish the first read before it starts
    system_memory_bootstrap();

    completion_init(&sampler.wake);
    sampler.thread = nd_thread_create("SYSMEM", NETDATA_THREAD_OPTION_MANUAL_JOIN, system_memory_sampler_thread, NULL);
    if(!sampler.thread)
        completion_destroy(&sampler.wake);
}

bool os_system_memory_sampler_stop(usec_t timeout_ut) {
    if(!sampler.thread)
        return !sampler.join_failed;

    if(nd_thread_is_me(sampler.thread))
        // a fatal on the sampler itself - it cannot wait for itself
        return false;

    nd_thread_signal_cancel(sampler.thread);

    usec_t until_ut = now_monotonic_usec() + timeout_ut;
    while(!__atomic_load_n(&sampler.exited, __ATOMIC_ACQUIRE)) {
        if(now_monotonic_usec() >= until_ut)
            // stuck in a read: abandon it
            return false;

        sleep_usec(10 * USEC_PER_MS);
    }

    if(nd_thread_join(sampler.thread) != 0) {
        // the join may have freed the handle anyway: never touch it again, and leave the thread behind
        sampler.thread = NULL;
        sampler.join_failed = true;
        return false;
    }

    sampler.thread = NULL;
    completion_destroy(&sampler.wake);
    return true;
}

// ----------------------------------------------------------------------------

double os_system_memory_available_percent(OS_SYSTEM_MEMORY mem) {
    if (!OS_SYSTEM_MEMORY_OK(mem))
        return 100.0;

    return 100.0 * (double)mem.ram_available_bytes / (double)mem.ram_total_bytes;
}

// ----------------------------------------------------------------------------
// unit test

static bool sm_equal(OS_SYSTEM_MEMORY a, OS_SYSTEM_MEMORY b) {
    return a.ram_total_bytes == b.ram_total_bytes && a.ram_available_bytes == b.ram_available_bytes;
}

#define SM_CHECK(cond, msg) do {                                        \
    if(!(cond)) { fprintf(stderr, "SYSTEM MEMORY: FAILED: %s\n", msg); errors++; } \
} while(0)

static bool sm_unittest_writer_stop = false;
static bool sm_unittest_writer_started = false;

// publishes samples whose two fields are always equal; a reader seeing them differ got a mixed sample
static void sm_unittest_writer(void *ptr __maybe_unused) {
    for(uint64_t v = 1; !__atomic_load_n(&sm_unittest_writer_stop, __ATOMIC_RELAXED); v++) {
        system_memory_publish((OS_SYSTEM_MEMORY){ .ram_total_bytes = v, .ram_available_bytes = v });
        if(v == 1)
            __atomic_store_n(&sm_unittest_writer_started, true, __ATOMIC_RELEASE);
    }
}

int os_system_memory_unittest(void) {
    int errors = 0;
    fprintf(stderr, "\nTesting system memory sampling...\n");

    // the published sample is process-wide: restore it at the end
    uint64_t saved_total = published.ram_total_bytes, saved_available = published.ram_available_bytes;
    usec_t saved_ut = published.sampled_ut;

    const OS_SYSTEM_MEMORY s1 = { .ram_total_bytes = 1000, .ram_available_bytes = 400 };
    system_memory_publish(s1);
    SM_CHECK(sm_equal(os_system_memory_cached(OS_SYSTEM_MEMORY_MAX_AGE_UT), s1), "a fresh sample is returned");
    SM_CHECK(sm_equal(os_system_memory_cached(0), s1), "a fresh sample is returned for any age");

    usec_t ut = published.sampled_ut;
    system_memory_publish(OS_SYSTEM_MEMORY_EMPTY);
    SM_CHECK(sm_equal(os_system_memory_cached(0), s1) && published.sampled_ut == ut,
             "a failed read keeps the previous values and sample time");

    system_memory_publish((OS_SYSTEM_MEMORY){ .ram_total_bytes = 0, .ram_available_bytes = 5 });
    SM_CHECK(sm_equal(os_system_memory_cached(0), s1) && published.sampled_ut == ut,
             "a sample without total memory is not published");

    __atomic_store_n(&published.sampled_ut, now_monotonic_usec() - OS_SYSTEM_MEMORY_MAX_AGE_UT - USEC_PER_SEC, __ATOMIC_RELEASE);
    SM_CHECK(!OS_SYSTEM_MEMORY_OK(os_system_memory_cached(OS_SYSTEM_MEMORY_MAX_AGE_UT)), "a stale sample is not returned as fresh");
    SM_CHECK(sm_equal(os_system_memory_cached(0), s1), "a stale sample is returned for any age");

    __atomic_store_n(&published.sampled_ut, 0, __ATOMIC_RELEASE);
    SM_CHECK(!OS_SYSTEM_MEMORY_OK(os_system_memory_cached(0)), "no sample returns empty");

    // a sample being written is never returned: the reader gives up and returns empty
    system_memory_publish(s1);
    __atomic_add_fetch(&published.seq, 1, __ATOMIC_RELEASE);
    SM_CHECK(!OS_SYSTEM_MEMORY_OK(os_system_memory_cached(0)), "a sample being written returns empty");
    __atomic_add_fetch(&published.seq, 1, __ATOMIC_RELEASE);
    SM_CHECK(sm_equal(os_system_memory_cached(0), s1), "a written sample is returned");

    // readers racing the writer must never get the fields of two different samples
    {
        __atomic_store_n(&sm_unittest_writer_stop, false, __ATOMIC_RELAXED);
        __atomic_store_n(&sm_unittest_writer_started, false, __ATOMIC_RELAXED);
        ND_THREAD *writer = nd_thread_create("SYSMEMTEST", NETDATA_THREAD_OPTION_MANUAL_JOIN, sm_unittest_writer, NULL);
        if(!writer) {
            fprintf(stderr, "SYSTEM MEMORY: FAILED: cannot create the writer thread\n");
            return 1;
        }

        for(int i = 0; i < 2000 && !__atomic_load_n(&sm_unittest_writer_started, __ATOMIC_ACQUIRE); i++)
            sleep_usec(USEC_PER_MS);

        if(!__atomic_load_n(&sm_unittest_writer_started, __ATOMIC_ACQUIRE)) {
            __atomic_store_n(&sm_unittest_writer_stop, true, __ATOMIC_RELAXED);
            nd_thread_join(writer);
            fprintf(stderr, "SYSTEM MEMORY: FAILED: the writer thread did not start\n");
            return 1;
        }

        // count the samples seen change, so that a pass needs reads overlapping the writer
        size_t reads = 0, mixed = 0, changes = 0;
        uint64_t last = 0;
        usec_t until_ut = now_monotonic_usec() + 500 * USEC_PER_MS;
        while(now_monotonic_usec() < until_ut) {
            OS_SYSTEM_MEMORY sm = os_system_memory_cached(0);
            if(!OS_SYSTEM_MEMORY_OK(sm))
                continue;

            reads++;
            if(sm.ram_total_bytes != sm.ram_available_bytes)
                mixed++;
            if(sm.ram_total_bytes != last) {
                changes++;
                last = sm.ram_total_bytes;
            }
        }

        __atomic_store_n(&sm_unittest_writer_stop, true, __ATOMIC_RELAXED);
        if(nd_thread_join(writer) != 0) {
            // the writer may still publish: the rest of the test cannot run
            fprintf(stderr, "SYSTEM MEMORY: FAILED: cannot join the writer thread\n");
            return 1;
        }

        fprintf(stderr, "SYSTEM MEMORY: %zu concurrent reads, %zu samples seen, %zu mixed\n", reads, changes, mixed);
        SM_CHECK(changes > 1, "concurrent reads overlap the writer");
        SM_CHECK(!mixed, "concurrent reads return whole samples");
    }

    // restore, then run the real sampler: it must publish a new sample and stop on request
    __atomic_store_n(&published.ram_total_bytes, saved_total, __ATOMIC_RELAXED);
    __atomic_store_n(&published.ram_available_bytes, saved_available, __ATOMIC_RELAXED);
    __atomic_store_n(&published.sampled_ut, saved_ut, __ATOMIC_RELEASE);

    os_system_memory_sampler_start();
    SM_CHECK(OS_SYSTEM_MEMORY_OK(os_system_memory()), "the first read returns system memory");

    usec_t before_ut = __atomic_load_n(&published.sampled_ut, __ATOMIC_ACQUIRE);
    for(int i = 0; i < 300 && __atomic_load_n(&published.sampled_ut, __ATOMIC_ACQUIRE) == before_ut; i++)
        sleep_usec(10 * USEC_PER_MS);
    SM_CHECK(__atomic_load_n(&published.sampled_ut, __ATOMIC_ACQUIRE) != before_ut, "the sampler publishes new samples");

    usec_t stop_ut = now_monotonic_usec();
    SM_CHECK(os_system_memory_sampler_stop(5 * USEC_PER_SEC), "the sampler stops on request");
    SM_CHECK(now_monotonic_usec() - stop_ut < 2 * USEC_PER_SEC, "the sampler stops without waiting for its tick");

    fprintf(stderr, "System memory sampling: %s\n", errors ? "FAILED" : "OK");
    return errors;
}
