// SPDX-License-Identifier: GPL-3.0-or-later

#include "nd-work-queue.h"

struct nd_work_queue_job {
    nd_work_queue_job_cb cb;
    void *data;
    struct nd_work_queue_job *prev, *next;
};

// An idle worker, waiting on its own condition variable. Idle workers are kept most-recent-first and a
// new job wakes the most recent one: under a steady trickle of jobs the same few threads keep working
// and the rest stay idle long enough to exit, so the pool shrinks to what the load needs. (With one
// shared condition variable every idle thread got a job now and then, and none ever timed out.)
struct nd_work_queue_waiter {
    netdata_cond_t cond;
    bool woken;     // set (under the mutex) by the waker, which also unlinks it
    struct nd_work_queue_waiter *prev, *next;
};

struct nd_work_queue {
    char tag[ND_THREAD_TAG_MAX + 1];

    nd_work_queue_thread_init_cb thread_init;
    void *thread_init_data;

    netdata_mutex_t mutex;
    netdata_cond_t exit_cond;   // a worker thread exited

    struct nd_work_queue_job *jobs; // FIFO, protected by mutex
    size_t queued;                  // jobs waiting for a thread
    struct nd_work_queue_waiter *waiters; // idle threads, most recent first, protected by mutex
    size_t idle;                    // threads in waiters
    size_t alive;                   // threads that have not exited yet
    bool stopping;

    size_t max_threads;
    usec_t idle_timeout_ut;         // an idle thread exits after this long without a job (0 = never)
    size_t spawned;                 // threads created so far, for naming them

    // The thread that exited last, not joined yet. Each exiting thread joins the one before it, so
    // at most one exited thread is ever left unjoined; submit and stop_and_free join that one.
    ND_THREAD *exited;
};

// called with the mutex held: returns the exited thread for the caller to join after unlocking
static ND_THREAD *nd_work_queue_take_exited_unsafe(ND_WORK_QUEUE *wq) {
    ND_THREAD *t = wq->exited;
    wq->exited = NULL;
    return t;
}

static void nd_work_queue_worker(void *arg) {
    ND_WORK_QUEUE *wq = arg;

    if(wq->thread_init)
        wq->thread_init(wq->thread_init_data);

    // linked into wq->waiters only while this thread is idle; its cond is reused for every idle period
    struct nd_work_queue_waiter waiter = { 0 };
    netdata_cond_init(&waiter.cond);

    netdata_mutex_lock(&wq->mutex);
    while(true) {
        struct nd_work_queue_job *job = wq->jobs;
        if(job) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wq->jobs, job, prev, next);
            wq->queued--;
            netdata_mutex_unlock(&wq->mutex);

            job->cb(job->data);
            freez(job);

            netdata_mutex_lock(&wq->mutex);
            continue;
        }

        if(wq->stopping)
            break;

        // idle: wait to be woken for a job, or until the idle timeout
        waiter.woken = false;
        DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(wq->waiters, &waiter, prev, next);
        wq->idle++;

        usec_t deadline_ut = wq->idle_timeout_ut ? now_monotonic_usec() + wq->idle_timeout_ut : 0;
        while(!waiter.woken) {
            if(!deadline_ut) {
                netdata_cond_wait(&waiter.cond, &wq->mutex);
                continue;
            }

            usec_t now_ut = now_monotonic_usec();
            if(now_ut >= deadline_ut)
                break;

            netdata_cond_timedwait(&waiter.cond, &wq->mutex, (deadline_ut - now_ut) * NSEC_PER_USEC);
        }

        bool timed_out = !waiter.woken;
        if(timed_out) {
            DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wq->waiters, &waiter, prev, next);
            wq->idle--;
        }

        // idle for too long, and nothing to run
        if(timed_out && !wq->jobs)
            break;

        // woken: run a job if there is one; another worker may have taken it, then wait again
    }

    // unlinked: a waker signals it only under the mutex, while it is linked
    netdata_cond_destroy(&waiter.cond);

    ND_THREAD *previous = nd_work_queue_take_exited_unsafe(wq);
    wq->exited = nd_thread_self();
    wq->alive--;
    netdata_cond_broadcast(&wq->exit_cond);
    netdata_mutex_unlock(&wq->mutex);

    // wq is not touched after the unlock: once alive is 0 stop_and_free may free it
    if(previous)
        nd_thread_join(previous);
}

ND_WORK_QUEUE *nd_work_queue_create(const char *tag, size_t max_threads, usec_t idle_timeout_ut,
                                    nd_work_queue_thread_init_cb thread_init, void *thread_init_data) {
    ND_WORK_QUEUE *wq = callocz(1, sizeof(*wq));
    strncpyz(wq->tag, tag, sizeof(wq->tag) - 1);
    wq->thread_init = thread_init;
    wq->thread_init_data = thread_init_data;
    wq->max_threads = max_threads ? max_threads : 1;
    wq->idle_timeout_ut = idle_timeout_ut;
    netdata_mutex_init(&wq->mutex);
    netdata_cond_init(&wq->exit_cond);
    return wq;
}

// called with the mutex held: a single attempt (NO_RETRY), so a system out of threads fails the
// submit at once instead of sleeping here; the submitter decides what to do with the job then
static void nd_work_queue_spawn_thread_unsafe(ND_WORK_QUEUE *wq) {
    char name[ND_THREAD_TAG_MAX + 1];
    snprintfz(name, sizeof(name) - 1, "%s[%zu]", wq->tag, wq->spawned);

    ND_THREAD *t = nd_thread_create(name, NETDATA_THREAD_OPTION_MANUAL_JOIN | NETDATA_THREAD_OPTION_DONT_LOG |
                                    NETDATA_THREAD_OPTION_NO_RETRY,
                                    nd_work_queue_worker, wq);
    if(!t)
        return;

    wq->spawned++;
    wq->alive++;
}

bool nd_work_queue_submit(ND_WORK_QUEUE *wq, nd_work_queue_job_cb cb, void *data) {
    if(!wq || !cb)
        return false;

    struct nd_work_queue_job *job = mallocz(sizeof(*job));
    job->cb = cb;
    job->data = data;
    job->prev = job->next = NULL;

    netdata_mutex_lock(&wq->mutex);

    if(wq->stopping) {
        netdata_mutex_unlock(&wq->mutex);
        freez(job);
        return false;
    }

    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(wq->jobs, job, prev, next);
    wq->queued++;

    // every queued job needs a thread of its own: wake the most recently idle one, or add one.
    // With neither (all threads busy, at max_threads), a busy thread takes it when it finishes:
    // a thread checks the queue before it goes idle.
    struct nd_work_queue_waiter *waiter = wq->waiters;
    if(waiter) {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wq->waiters, waiter, prev, next);
        wq->idle--;
        waiter->woken = true;
        netdata_cond_signal(&waiter->cond);
    }
    else if(wq->alive < wq->max_threads)
        nd_work_queue_spawn_thread_unsafe(wq);

    bool accepted = true;
    if(!wq->alive) {
        // no thread exists and none could be created: nothing would ever run this job
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wq->jobs, job, prev, next);
        wq->queued--;
        accepted = false;
    }

    ND_THREAD *exited = nd_work_queue_take_exited_unsafe(wq);
    netdata_mutex_unlock(&wq->mutex);

    // it has already left the queue; joining only reaps it
    if(exited)
        nd_thread_join(exited);

    if(!accepted)
        freez(job);

    return accepted;
}

void nd_work_queue_stop(ND_WORK_QUEUE *wq) {
    if(!wq)
        return;

    netdata_mutex_lock(&wq->mutex);
    wq->stopping = true;

    // wake every idle thread: each runs what is still queued, then exits
    while(wq->waiters) {
        struct nd_work_queue_waiter *waiter = wq->waiters;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(wq->waiters, waiter, prev, next);
        wq->idle--;
        waiter->woken = true;
        netdata_cond_signal(&waiter->cond);
    }
    netdata_mutex_unlock(&wq->mutex);
}

bool nd_work_queue_stop_and_free(ND_WORK_QUEUE *wq, usec_t timeout_ut) {
    if(!wq)
        return true;

    nd_work_queue_stop(wq);

    usec_t deadline_ut = now_monotonic_usec() + timeout_ut;

    netdata_mutex_lock(&wq->mutex);
    while(wq->alive) {
        usec_t now_ut = now_monotonic_usec();
        if(now_ut >= deadline_ut)
            break;

        netdata_cond_timedwait(&wq->exit_cond, &wq->mutex, (deadline_ut - now_ut) * NSEC_PER_USEC);
    }
    size_t alive = wq->alive;
    ND_THREAD *exited = alive ? NULL : nd_work_queue_take_exited_unsafe(wq);
    netdata_mutex_unlock(&wq->mutex);

    if(alive) {
        nd_log_daemon(NDLP_WARNING,
                      "WORK QUEUE '%s': %zu thread(s) still running a job after %"PRIu64" ms, not freeing the queue",
                      wq->tag, alive, (uint64_t)(timeout_ut / USEC_PER_MS));
        return false;
    }

    // every thread has left the queue; each one joined the one that exited before it, so joining
    // the last one reaps them all
    if(exited)
        nd_thread_join(exited);

    netdata_cond_destroy(&wq->exit_cond);
    netdata_mutex_destroy(&wq->mutex);
    freez(wq);
    return true;
}

// --------------------------------------------------------------------------------------------------------------------
// unittest

struct nd_work_queue_ut {
    size_t ran;
    size_t running;
    size_t max_running;
    size_t init_calls;
    struct completion release;  // jobs that block wait here
    bool block;
};

static void nd_work_queue_ut_init(void *data) {
    struct nd_work_queue_ut *ut = data;
    __atomic_add_fetch(&ut->init_calls, 1, __ATOMIC_RELAXED);
}

static void nd_work_queue_ut_job(void *data) {
    struct nd_work_queue_ut *ut = data;
    size_t running = __atomic_add_fetch(&ut->running, 1, __ATOMIC_RELAXED);

    size_t max = __atomic_load_n(&ut->max_running, __ATOMIC_RELAXED);
    while(running > max && !__atomic_compare_exchange_n(&ut->max_running, &max, running, false, __ATOMIC_RELAXED, __ATOMIC_RELAXED))
        ;

    if(__atomic_load_n(&ut->block, __ATOMIC_RELAXED))
        completion_wait_for(&ut->release);

    __atomic_sub_fetch(&ut->running, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ut->ran, 1, __ATOMIC_RELAXED);
}

static bool nd_work_queue_ut_wait_for(size_t *counter, size_t expected, usec_t timeout_ut) {
    usec_t deadline_ut = now_monotonic_usec() + timeout_ut;
    while(__atomic_load_n(counter, __ATOMIC_RELAXED) != expected) {
        if(now_monotonic_usec() >= deadline_ut)
            return false;
        sleep_usec(1000);
    }
    return true;
}

int nd_work_queue_unittest(void) {
    int errors = 0;
    fprintf(stderr, "\nTesting the work queue...\n");

    // 1. blocking jobs run in parallel up to max_threads, the rest queue; nothing is lost
    {
        struct nd_work_queue_ut ut = { .block = true };
        completion_init(&ut.release);
        ND_WORK_QUEUE *wq = nd_work_queue_create("WQUT", 4, 0, nd_work_queue_ut_init, &ut);

        for(size_t i = 0; i < 10; i++) {
            if(!nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut)) {
                fprintf(stderr, "  FAILED: submit %zu was refused\n", i);
                errors++;
            }
        }

        if(!nd_work_queue_ut_wait_for(&ut.running, 4, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: expected 4 jobs running in parallel, got %zu\n",
                    __atomic_load_n(&ut.running, __ATOMIC_RELAXED));
            errors++;
        }

        completion_mark_complete(&ut.release);
        if(!nd_work_queue_ut_wait_for(&ut.ran, 10, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: expected 10 jobs to run, got %zu\n", __atomic_load_n(&ut.ran, __ATOMIC_RELAXED));
            errors++;
        }

        if(ut.max_running > 4) {
            fprintf(stderr, "  FAILED: %zu jobs ran in parallel, the limit is 4\n", ut.max_running);
            errors++;
        }

        if(!nd_work_queue_stop_and_free(wq, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: the queue did not stop\n");
            errors++;
        }

        if(ut.init_calls != 4) {
            fprintf(stderr, "  FAILED: expected 4 thread init calls, got %zu\n", ut.init_calls);
            errors++;
        }
        completion_destroy(&ut.release);
    }

    // 2. threads are created on demand: one job at a time needs one thread
    {
        struct nd_work_queue_ut ut = { 0 };
        ND_WORK_QUEUE *wq = nd_work_queue_create("WQUT", 8, 0, nd_work_queue_ut_init, &ut);
        for(size_t i = 0; i < 5; i++) {
            nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut);
            if(!nd_work_queue_ut_wait_for(&ut.ran, i + 1, 10 * USEC_PER_SEC)) {
                fprintf(stderr, "  FAILED: sequential job %zu did not run\n", i);
                errors++;
            }

            // the worker counts as idle only once it waits for a job again: the next submit must find it so
            usec_t deadline_ut = now_monotonic_usec() + 10 * USEC_PER_SEC;
            while(true) {
                netdata_mutex_lock(&wq->mutex);
                size_t idle = wq->idle;
                netdata_mutex_unlock(&wq->mutex);
                if(idle == 1 || now_monotonic_usec() >= deadline_ut)
                    break;
                sleep_usec(1000);
            }
        }
        nd_work_queue_stop_and_free(wq, 10 * USEC_PER_SEC);
        if(ut.init_calls != 1) {
            fprintf(stderr, "  FAILED: sequential jobs created %zu threads, expected 1\n", ut.init_calls);
            errors++;
        }
    }

    // 3. accepted jobs still run after stop; new jobs are refused
    {
        struct nd_work_queue_ut ut = { .block = true };
        completion_init(&ut.release);
        ND_WORK_QUEUE *wq = nd_work_queue_create("WQUT", 1, 0, NULL, NULL);
        for(size_t i = 0; i < 3; i++) {
            if(!nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut)) {
                fprintf(stderr, "  FAILED: submit %zu was refused\n", i);
                errors++;
            }
        }

        if(!nd_work_queue_ut_wait_for(&ut.running, 1, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: expected 1 job running, got %zu\n",
                    __atomic_load_n(&ut.running, __ATOMIC_RELAXED));
            errors++;
        }
        nd_work_queue_stop(wq);

        if(nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut)) {
            fprintf(stderr, "  FAILED: a stopped queue accepted a job\n");
            errors++;
        }

        // a job is still running: freeing must time out and leave the queue alone
        bool freed = nd_work_queue_stop_and_free(wq, 100 * USEC_PER_MS);
        if(freed) {
            fprintf(stderr, "  FAILED: the queue was freed while a job was running\n");
            errors++;
        }

        completion_mark_complete(&ut.release);
        if(!freed && !nd_work_queue_stop_and_free(wq, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: the queue did not stop after its jobs finished\n");
            errors++;
        }

        if(ut.ran != 3) {
            fprintf(stderr, "  FAILED: expected the 3 accepted jobs to run, got %zu\n", ut.ran);
            errors++;
        }
        completion_destroy(&ut.release);
    }

    // 4. idle threads exit after the idle timeout; the next submit creates a new one
    {
        struct nd_work_queue_ut ut = { 0 };
        ND_WORK_QUEUE *wq = nd_work_queue_create("WQUT", 4, 50 * USEC_PER_MS, nd_work_queue_ut_init, &ut);
        for(size_t round = 1; round <= 3; round++) {
            for(size_t i = 0; i < 3; i++)
                nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut);

            if(!nd_work_queue_ut_wait_for(&ut.ran, round * 3, 10 * USEC_PER_SEC)) {
                fprintf(stderr, "  FAILED: round %zu: jobs did not run\n", round);
                errors++;
            }

            usec_t deadline_ut = now_monotonic_usec() + 10 * USEC_PER_SEC;
            size_t alive;
            while(true) {
                netdata_mutex_lock(&wq->mutex);
                alive = wq->alive;
                netdata_mutex_unlock(&wq->mutex);
                if(!alive || now_monotonic_usec() >= deadline_ut)
                    break;
                sleep_usec(10 * USEC_PER_MS);
            }
            if(alive) {
                fprintf(stderr, "  FAILED: round %zu: %zu idle thread(s) did not exit\n", round, alive);
                errors++;
            }
        }

        if(ut.init_calls < 3) {
            fprintf(stderr, "  FAILED: expected new threads after the idle ones exited, got %zu thread starts\n", ut.init_calls);
            errors++;
        }

        if(!nd_work_queue_stop_and_free(wq, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: the queue did not stop\n");
            errors++;
        }
    }

    // 5. under a steady trickle of jobs the pool shrinks to what the load needs: the most recently idle
    //    thread gets each job, the others stay idle and exit
    {
        struct nd_work_queue_ut ut = { .block = true };
        completion_init(&ut.release);
        ND_WORK_QUEUE *wq = nd_work_queue_create("WQUT", 4, 200 * USEC_PER_MS, nd_work_queue_ut_init, &ut);

        // occupy 4 threads at once, then let them all go idle
        for(size_t i = 0; i < 4; i++)
            nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut);
        if(!nd_work_queue_ut_wait_for(&ut.running, 4, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: expected 4 jobs running in parallel, got %zu\n",
                    __atomic_load_n(&ut.running, __ATOMIC_RELAXED));
            errors++;
        }
        __atomic_store_n(&ut.block, false, __ATOMIC_RELAXED);
        completion_mark_complete(&ut.release);

        // the 4 threads must be seen idle before they time out, or the trickle below would not test the wake order
        usec_t deadline_ut = now_monotonic_usec() + 10 * USEC_PER_SEC;
        size_t idle;
        while(true) {
            netdata_mutex_lock(&wq->mutex);
            idle = wq->idle;
            netdata_mutex_unlock(&wq->mutex);
            if(idle == 4 || now_monotonic_usec() >= deadline_ut)
                break;
            sleep_usec(1000);
        }
        if(idle != 4) {
            fprintf(stderr, "  FAILED: expected 4 idle threads before the trickle, saw %zu\n", idle);
            errors++;
        }

        // one short job at a time, every 20 ms - more often than the idle timeout - for up to 10 s
        size_t expected = 4, alive = 4;
        deadline_ut = now_monotonic_usec() + 10 * USEC_PER_SEC;
        while(now_monotonic_usec() < deadline_ut) {
            nd_work_queue_submit(wq, nd_work_queue_ut_job, &ut);
            expected++;
            if(!nd_work_queue_ut_wait_for(&ut.ran, expected, 10 * USEC_PER_SEC)) {
                fprintf(stderr, "  FAILED: trickle job %zu did not run\n", expected);
                errors++;
                break;
            }

            netdata_mutex_lock(&wq->mutex);
            alive = wq->alive;
            netdata_mutex_unlock(&wq->mutex);
            if(alive == 1)
                break;

            sleep_usec(20 * USEC_PER_MS);
        }

        if(alive != 1) {
            fprintf(stderr, "  FAILED: under a trickle of jobs %zu threads stayed alive, expected 1\n", alive);
            errors++;
        }

        if(!nd_work_queue_stop_and_free(wq, 10 * USEC_PER_SEC)) {
            fprintf(stderr, "  FAILED: the queue did not stop\n");
            errors++;
        }
        completion_destroy(&ut.release);
    }

    fprintf(stderr, "Work queue tests: %s\n", errors ? "FAILED" : "OK");
    return errors;
}
