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

            if (Exit)
                pthread_exit(nullptr);
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
