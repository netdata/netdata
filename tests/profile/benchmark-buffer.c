/* SPDX-License-Identifier: GPL-3.0-or-later */

#include "config.h"
#include "libnetdata/libnetdata.h"
#include <pthread.h>
#include <stdlib.h>

#define NUMTHREADS 8

pthread_t threads[NUMTHREADS];

//ugly workaround for missing symbol
void send_statistics(__attribute__((unused)) const char *action, __attribute__((unused)) const char *action_result, __attribute__((unused)) const char *action_data){
}

void netdata_cleanup_and_exit(int ret) { exit(ret); }

pthread_mutex_t mprintf_lock = PTHREAD_MUTEX_INITIALIZER;
#define muprintf(...) { pthread_mutex_lock(&mprintf_lock); printf (__VA_ARGS__); fflush(stdout); pthread_mutex_unlock(&mprintf_lock); }

pthread_barrier_t next_test_barrier;
void threads_sync_next_test_start(){
    pthread_barrier_wait(&next_test_barrier);
}
#define threads_synce_test_end threads_sync_next_test_start

typedef struct {
    char cmd;
    size_t size;
} test_command;

//based on real life scenario
//test run of full netdata and thread WEB static 3
test_command real_life_scenario_1[] = {
    { '+',  16384 },
    { '+',  4096 },
    { '+',  4096 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '-',  100 },
    { '+',  100 },
    { '+', 100 },
    { '-', 100 },
    { '-', 4096 },
    { '-', 4096 },
    { '+', 16384 },
    { '+', 4096 },
    { '+', 4096 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '+', 100 },
    { '-', 100 },
    { '-', 4096 },
    { '-', 16384 },
    { '-', 4096 },
    { '-', 16384 },
    { 0, 0}
};

#define REAL_LIFE_TEST_BUFFER_MAX_COUNT 100
#define REAL_LIFE_TEST_REPEAT_COUNT 250000

int real_life_test_insert(BUFFER **stack, size_t stack_size, BUFFER* buffer) {
    if(!buffer || !stack)
        return 1;
    for(size_t i = 0; i < stack_size; i++){
        if(!stack[i]) {
            stack[i] = buffer;
            return 0;
        }
    }
    return 1;
}

void *real_life_sceario_test( void *name ) {
    BUFFER **stack = calloc(REAL_LIFE_TEST_BUFFER_MAX_COUNT, sizeof(BUFFER*));

    for(int repeat = 0; repeat < REAL_LIFE_TEST_REPEAT_COUNT; repeat++) {
        int i = 0;
        while(real_life_scenario_1[i].cmd) {
            if(real_life_scenario_1[i].cmd == '+') {
                BUFFER *new = buffer_create(real_life_scenario_1[i].size);
                if(real_life_test_insert(stack, REAL_LIFE_TEST_BUFFER_MAX_COUNT, new)) {
                    muprintf("%s: Out of space in working buffer.\n", (char*)name);
                    exit(1);
                }
            } else if (real_life_scenario_1[i].cmd == '-') {
                for(int j = 0; j < REAL_LIFE_TEST_BUFFER_MAX_COUNT; j++){
                    if(stack[j] && stack[j]->size == real_life_scenario_1[i].size){
                        buffer_free(stack[j]);
                        stack[j] = NULL;
                    }
                }
            }
            i++;
        }
    }

    for(int j = 0; j < REAL_LIFE_TEST_BUFFER_MAX_COUNT; j++)
        if(stack[j])
            buffer_free(stack[j]);

    free((void*) stack);
    return NULL;
}

#define TEST_SYNTH_SIMPLE_COUNT     10000000
#define TEST_SYNTH_SIMPLE_BUFSIZE   256
void *synthetic_test_simple( void *tname ) {
    BUFFER* buffer;
    for(long long int i = 0; i < TEST_SYNTH_SIMPLE_COUNT; i++) {
        buffer = buffer_create(TEST_SYNTH_SIMPLE_BUFSIZE);
        buffer_snprintf(buffer, TEST_SYNTH_SIMPLE_BUFSIZE, "Test1: %s", (char*)tname);
        buffer_free(buffer);
    }
    return NULL;
}

typedef void *(*test_fnc)(void*);
typedef struct {
    test_fnc fnc;
    char* description;
} test_run_definition;

test_run_definition test_list[] = {
    { synthetic_test_simple, "Synthetic" },
    { real_life_sceario_test, "Real life based" },
    { synthetic_test_simple, "Synthetic Run 2" },
    { real_life_sceario_test, "Real life based Run 2" },
    { NULL, NULL }
};

void *thread_test_worker( void *ptr ) {
    int i = 0;
    while(test_list[i].fnc) {
        threads_sync_next_test_start();
        test_list[i].fnc(ptr);
        threads_synce_test_end();
        i++;
    }
    free(ptr);
    return NULL;
}

#define TNAME_BUFSIZE 100
int main(__attribute__((unused)) int argc, __attribute__((unused)) char **argv) {
    printf("Benchmarking netdata Buffer.c\n\tThreads: %d\n", NUMTHREADS);
    printf("\tMempool is: %s\n", buffer_mempool_status());

    pthread_mutex_init(&mprintf_lock, NULL);
    pthread_barrier_init(&next_test_barrier, NULL, NUMTHREADS+1); //+1 is for main - to be able to synchronize tests

    char *tname = NULL;
    for(int i = 0; i < NUMTHREADS; i++) {
        tname = calloc(1, TNAME_BUFSIZE);
        snprintf(tname, TNAME_BUFSIZE, "ThreadIdx%02d", i);
        if(pthread_create(&threads[i], NULL, thread_test_worker, (void*)tname)) {
            printf("Failed to create thread %d.\n", i);
            exit(1);
        }
    }

    int i = 0;
    while(test_list[i].fnc) {
        muprintf("Starting Test %d (\"%s\")\n", i+1, test_list[i].description);
        threads_sync_next_test_start();
        threads_synce_test_end();
        i++;
    }

    for(int i = 0; i < NUMTHREADS; i++) {
        if(pthread_join(threads[i], NULL)) {
            printf("Failed to join thread %d.\n", i);
            exit(1);
        }
    }

    printf("ALL SUCCESSFUL.\n");
    exit(0);
}
