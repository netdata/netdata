// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * A very simple pthreads program to spawn N busy threads.
 * It is just used for validating apps.plugin CPU utilization
 * calculations per operating system.
 *
 * Compile with:
 *
 * gcc -O2 -ggdb -o busy_threads busy_threads.c -pthread
 *
 * Run as:
 *
 * busy_threads 2
 *
 * The above will create 2 busy threads, each using 1 core in user time.
 *
 */

#include <stdio.h>
#include <stdlib.h>
#include <pthread.h>
#include <signal.h>
#include <unistd.h>

volatile int keep_running = 1;

void handle_signal(int signal) {
    keep_running = 0;
}

void *busy_loop(void *arg) {
    while (keep_running) {
        // Busy loop to keep CPU at 100%
    }
    return NULL;
}

int main(int argc, char *argv[]) {
    if (argc != 2) {
        fprintf(stderr, "Usage: %s <number of threads>\n", argv[0]);
        exit(EXIT_FAILURE);
    }

    int num_threads = atoi(argv[1]);
    if (num_threads <= 0) {
        fprintf(stderr, "Number of threads must be a positive integer.\n");
        exit(EXIT_FAILURE);
    }

    // Register the signal handler to gracefully exit on Ctrl-C
    signal(SIGINT, handle_signal);

    pthread_t *threads = malloc(sizeof(pthread_t) * num_threads);
    if (threads == NULL) {
        perror("malloc");
        exit(EXIT_FAILURE);
    }

    // Create threads
    for (int i = 0; i < num_threads; i++) {
        if (pthread_create(&threads[i], NULL, busy_loop, NULL) != 0) {
            perror("pthread_create");
            free(threads);
            exit(EXIT_FAILURE);
        }
    }

    // Wait for threads to finish (they never will unless interrupted)
    for (int i = 0; i < num_threads; i++) {
        pthread_join(threads[i], NULL);
    }

    free(threads);
    return 0;
}
