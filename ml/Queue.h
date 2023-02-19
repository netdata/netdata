#ifndef QUEUE_H
#define QUEUE_H

#include "ml-private.h"
#include "Mutex.h"
#include <queue>
#include <mutex>
#include <condition_variable>

template<typename T>
class Queue {
public:
    Queue(void) : Q(), M() {
        pthread_cond_init(&CV, nullptr);
        Exit = false;
    }

    ~Queue() {
        pthread_cond_destroy(&CV);
    }

    void push(T t) {
        std::lock_guard<Mutex> L(M);

        Q.push(t);
        pthread_cond_signal(&CV);
    }

    std::pair<T, size_t> pop(void) {
        std::lock_guard<Mutex> L(M);

        while (Q.empty()) {
            pthread_cond_wait(&CV, M.inner());

            if (Exit) {
                // This should happen only when we are destroying a host.
                // Callers should use a flag dedicated to checking if we
                // are about to delete the host or exit the agent. The original
                // implementation would call pthread_exit which would cause
                // the queue's mutex to be destroyed twice (and fail on the
                // 2nd time)
                return { T(), 0 };
            }
        }

        T V = Q.front();
        size_t Size = Q.size();
        Q.pop();

        return { V, Size };
    }

    void signal() {
        std::lock_guard<Mutex> L(M);
        Exit = true;
        pthread_cond_signal(&CV);
    }

private:
    std::queue<T> Q;
    Mutex M;
    pthread_cond_t CV;
    std::atomic<bool> Exit;
};

#endif /* QUEUE_H */
