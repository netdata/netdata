// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef BIT_RATE_WINDOW_H
#define BIT_RATE_WINDOW_H

#include "BitBufferCounter.h"
#include "ml-private.h"

namespace ml {

class BitRateWindow {
public:
    enum class State {
        NotFilled,
        BelowThreshold,
        AboveThreshold,
        Idle
    };

    using Edge = std::pair<State, State>;
    using Action = size_t (BitRateWindow::*)(State PrevState, bool NewBit);

private:
    std::map<Edge, Action> EdgeActions = {
        // From == To
        {
            Edge(State::NotFilled, State::NotFilled),
            &BitRateWindow::onRoundtripNotFilled,
        },
        {
            Edge(State::BelowThreshold, State::BelowThreshold),
            &BitRateWindow::onRoundtripBelowThreshold,
        },
        {
            Edge(State::AboveThreshold, State::AboveThreshold),
            &BitRateWindow::onRoundtripAboveThreshold,
        },
        {
            Edge(State::Idle, State::Idle),
            &BitRateWindow::onRoundtripIdle,
        },


        // NotFilled => {BelowThreshold, AboveThreshold}
        {
            Edge(State::NotFilled, State::BelowThreshold),
            &BitRateWindow::onNotFilledToBelowThreshold
        },
        {
            Edge(State::NotFilled, State::AboveThreshold),
            &BitRateWindow::onNotFilledToAboveThreshold
        },

        // BelowThreshold => AboveThreshold
        {
            Edge(State::BelowThreshold, State::AboveThreshold),
            &BitRateWindow::onBelowToAboveThreshold
        },

        // AboveThreshold => Idle
        {
            Edge(State::AboveThreshold, State::Idle),
            &BitRateWindow::onAboveThresholdToIdle
        },

        // Idle => NotFilled
        {
            Edge(State::Idle, State::NotFilled),
            &BitRateWindow::onIdleToNotFilled
        },
    };

public:
    BitRateWindow(size_t MinLength, size_t MaxLength, size_t IdleLength,
                  size_t SetBitsThreshold) :
        MinLength(MinLength), MaxLength(MaxLength), IdleLength(IdleLength),
        SetBitsThreshold(SetBitsThreshold),
        CurrState(State::NotFilled), CurrLength(0), BBC(MinLength) {}

    std::pair<Edge, size_t> insert(bool Bit);

    void print(std::ostream &OS) const;

private:
    size_t onRoundtripNotFilled(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength += 1;
        return CurrLength;
    }

    size_t onRoundtripBelowThreshold(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength = MinLength;
        return CurrLength;
    }

    size_t onRoundtripAboveThreshold(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength += 1;
        return CurrLength;
    }

    size_t onRoundtripIdle(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength += 1;
        return CurrLength;
    }

    size_t onNotFilledToBelowThreshold(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength = MinLength;
        return CurrLength;
    }

    size_t onNotFilledToAboveThreshold(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength += 1;
        return CurrLength;
    }

    size_t onBelowToAboveThreshold(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        CurrLength = MinLength;
        return CurrLength;
    }

    size_t onAboveThresholdToIdle(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        size_t PrevLength = CurrLength;
        CurrLength = 1;
        return PrevLength;
    }

    size_t onIdleToNotFilled(State PrevState, bool NewBit) {
        (void) PrevState, (void) NewBit;

        BBC = BitBufferCounter(MinLength);
        BBC.insert(NewBit);

        CurrLength = 1;
        return CurrLength;
    }

private:
    size_t MinLength;
    size_t MaxLength;
    size_t IdleLength;
    size_t SetBitsThreshold;

    State CurrState;
    size_t CurrLength;
    BitBufferCounter BBC;
};

} // namespace ml

inline std::ostream& operator<<(std::ostream &OS, const ml::BitRateWindow BRW) {
    BRW.print(OS);
    return OS;
}

#endif /* BIT_RATE_WINDOW_H */
