// SPDX-License-Identifier: GPL-3.0-or-later

#include "BitRateWindow.h"

using namespace ml;

std::pair<BitRateWindow::Edge, size_t> BitRateWindow::insert(bool Bit) {
    Edge E;

    BBC.insert(Bit);
    switch (CurrState) {
        case State::NotFilled: {
            if (BBC.isFilled()) {
                if (BBC.numSetBits() < SetBitsThreshold) {
                    CurrState = State::BelowThreshold;
                } else {
                    CurrState = State::AboveThreshold;
                }
            } else {
                CurrState = State::NotFilled;
            }

            E = {State::NotFilled, CurrState};
            break;
        } case State::BelowThreshold: {
            if (BBC.numSetBits() >= SetBitsThreshold) {
                CurrState = State::AboveThreshold;
            }

            E = {State::BelowThreshold, CurrState};
            break;
        } case State::AboveThreshold: {
            if ((BBC.numSetBits() < SetBitsThreshold) ||
                (CurrLength == MaxLength)) {
                CurrState = State::Idle;
            }

            E = {State::AboveThreshold, CurrState};
            break;
        } case State::Idle: {
            if (CurrLength == IdleLength) {
                CurrState = State::NotFilled;
            }

            E = {State::Idle, CurrState};
            break;
        }
    }

    Action A = EdgeActions[E];
    size_t L = (this->*A)(E.first, Bit);
    return {E, L};
}

void BitRateWindow::print(std::ostream &OS) const {
    switch (CurrState) {
        case State::NotFilled:
            OS << "NotFilled";
            break;
        case State::BelowThreshold:
            OS << "BelowThreshold";
            break;
        case State::AboveThreshold:
            OS << "AboveThreshold";
            break;
        case State::Idle:
            OS << "Idle";
            break;
        default:
            OS << "UnknownState";
            break;
    }

    OS << ": " << BBC << " (Current Length: " << CurrLength << ")";
}
