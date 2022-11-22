#ifndef QUEUE_H
#define QUEUE_H

#include <queue>
#include <mutex>
#include <condition_variable>

template<typename T>
class Queue {
public:
    Queue(void) : Q(), Mutex(), CondVar() { }

    void push(T t) {
        std::lock_guard<std::mutex> Lock(Mutex);
        Q.push(t);
        CondVar.notify_one();
    }

    std::pair<T, size_t> pop(void) {
        std::unique_lock<std::mutex> Lock(Mutex);
        while (Q.empty())
            CondVar.wait(Lock);

        T V = Q.front();
        size_t Size = Q.size();

        Q.pop();
        return { V, Size };
    }

private:
    std::queue<T> Q;
    std::mutex Mutex;
    std::condition_variable CondVar;
};

#endif /* QUEUE_H */
