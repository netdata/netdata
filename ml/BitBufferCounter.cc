// SPDX-License-Identifier: GPL-3.0-or-later

#include "BitBufferCounter.h"

using namespace ml;

std::vector<bool> BitBufferCounter::getBuffer() const {
    std::vector<bool> Buffer;

    for (size_t Idx = start(); Idx != (start() + size()); Idx++)
        Buffer.push_back(V[Idx % V.size()]);

    return Buffer;
}

void BitBufferCounter::insert(bool Bit) {
    if (N >= V.size())
        NumSetBits -= (V[start()] == true);

    NumSetBits += (Bit == true);
    V[N++ % V.size()] = Bit;
}

void BitBufferCounter::print(std::ostream &OS) const {
    std::vector<bool> Buffer = getBuffer();

    for (bool B : Buffer)
        OS << B;
}
