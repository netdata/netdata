// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef BIT_BUFFER_COUNTER_H
#define BIT_BUFFER_COUNTER_H

#include "ml-private.h"

namespace ml {

class BitBufferCounter {
public:
    BitBufferCounter(size_t Capacity) : V(Capacity, 0), NumSetBits(0), N(0) {}

    std::vector<bool> getBuffer() const;

    void insert(bool Bit);

    void print(std::ostream &OS) const;

    bool isFilled() const {
       return N >= V.size();
    }

    size_t numSetBits() const {
        return NumSetBits;
    }

private:
    inline size_t size() const {
        return N < V.size() ? N : V.size();
    }

    inline size_t start() const {
        if (N <= V.size())
            return 0;

        return N % V.size();
    }

private:
    std::vector<bool> V;
    size_t NumSetBits;

    size_t N;
};

} // namespace ml

inline std::ostream& operator<<(std::ostream &OS, const ml::BitBufferCounter &BBC) {
    BBC.print(OS);
    return OS;
}

#endif /* BIT_BUFFER_COUNTER_H */
