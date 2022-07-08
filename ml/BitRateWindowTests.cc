// SPDX-License-Identifier: GPL-3.0-or-later

#include "BitBufferCounter.h"
#include "BitRateWindow.h"

#include "gtest/gtest.h"

using namespace ml;

TEST(BitBufferCounterTest, Cap_4) {
    size_t Capacity = 4;
    BitBufferCounter BBC(Capacity);

    // No bits set
    EXPECT_EQ(BBC.numSetBits(), 0);

    // All ones
    for (size_t Idx = 0; Idx != (2 * Capacity); Idx++) {
        BBC.insert(true);

        EXPECT_EQ(BBC.numSetBits(), std::min(Idx + 1, Capacity));
    }

    // All zeroes
    for (size_t Idx = 0; Idx != Capacity; Idx++) {
        BBC.insert(false);

        if (Idx < Capacity)
            EXPECT_EQ(BBC.numSetBits(), Capacity - (Idx + 1));
        else
            EXPECT_EQ(BBC.numSetBits(), 0);
    }

    // Even ones/zeroes
    for (size_t Idx = 0; Idx != (2 * Capacity); Idx++)
        BBC.insert(Idx % 2 == 0);
    EXPECT_EQ(BBC.numSetBits(), Capacity / 2);
}

using State = BitRateWindow::State;
using Edge = BitRateWindow::Edge;
using Result = std::pair<Edge, size_t>;

TEST(BitRateWindowTest, Cycles) {
    /* Test the FSM by going through its two cycles:
     *  1) NotFilled -> AboveThreshold -> Idle -> NotFilled
     *  2) NotFilled -> BelowThreshold -> AboveThreshold -> Idle -> NotFilled
     *
     * Check the window's length on every new state transition.
     */

    size_t MinLength = 4, MaxLength = 6, IdleLength = 5;
    size_t SetBitsThreshold = 3;

    Result R;
    BitRateWindow BRW(MinLength, MaxLength, IdleLength, SetBitsThreshold);

    /*
     * 1st cycle
     */

    // NotFilled -> AboveThreshold
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);

    // AboveThreshold -> Idle
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::AboveThreshold));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::AboveThreshold));

    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::Idle));
    EXPECT_EQ(R.second, MaxLength);


    // Idle -> NotFilled
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::NotFilled));
    EXPECT_EQ(R.second, 1);

    // NotFilled -> AboveThreshold
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);

    /*
     * 2nd cycle
     */

    BRW = BitRateWindow(MinLength, MaxLength, IdleLength, SetBitsThreshold);

    // NotFilled -> BelowThreshold
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::BelowThreshold));
    EXPECT_EQ(R.second, MinLength);

    // BelowThreshold -> BelowThreshold:
    //      Check the state's self loop by adding set bits that will keep the
    //      bit buffer below the specified threshold.
    //
    for (size_t Idx = 0; Idx != 2 * MaxLength; Idx++) {
        R = BRW.insert(Idx % 2 == 0);
        EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::BelowThreshold));
        EXPECT_EQ(R.second, MinLength);
    }

    // Verify that at the end of the loop the internal bit buffer contains
    // "1010". Do so by adding one set bit and checking that we remain below
    // the specified threshold.
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::BelowThreshold));
    EXPECT_EQ(R.second, MinLength);

    // BelowThreshold -> AboveThreshold
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);

    // AboveThreshold -> Idle:
    //      Do the transition without filling the max window size this time.
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::Idle));
    EXPECT_EQ(R.second, MinLength);

    // Idle -> NotFilled
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::NotFilled));
    EXPECT_EQ(R.second, 1);

    // NotFilled -> AboveThreshold
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::NotFilled));
    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);
}

TEST(BitRateWindowTest, ConsecutiveOnes) {
    size_t MinLength = 120, MaxLength = 240, IdleLength = 30;
    size_t SetBitsThreshold = 30;

    Result R;
    BitRateWindow BRW(MinLength, MaxLength, IdleLength, SetBitsThreshold);

    for (size_t Idx = 0; Idx != MaxLength; Idx++)
        R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::BelowThreshold));
    EXPECT_EQ(R.second, MinLength);

    for (size_t Idx = 0; Idx != SetBitsThreshold; Idx++) {
        EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::BelowThreshold));
        R = BRW.insert(true);
    }
    EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);

    // At this point the window's buffer contains:
    //      (MinLength - SetBitsThreshold = 90) 0s, followed by
    //                  (SetBitsThreshold = 30) 1s.
    //
    // To go below the threshold, we need to add (90 + 1) more 0s in the window's
    // buffer. At that point, the the window's buffer will contain:
    //                  (SetBitsThreshold = 29) 1s, followed by
    //      (MinLength - SetBitsThreshold = 91) 0s.
    //
    // Right before adding the last 0, we expect the window's length to be equal to 210,
    // because the bit buffer has gone through these bits:
    //      (MinLength - SetBitsThreshold = 90) 0s, followed by
    //                  (SetBitsThreshold = 30) 1s, followed by
    //      (MinLength - SetBitsThreshold = 90) 0s.

    for (size_t Idx = 0; Idx != (MinLength - SetBitsThreshold); Idx++) {
        R = BRW.insert(false);
        EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::AboveThreshold));
    }
    EXPECT_EQ(R.second, 2 * MinLength - SetBitsThreshold);
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::Idle));

    // Continue with the Idle -> NotFilled edge.
    for (size_t Idx = 0; Idx != IdleLength - 1; Idx++) {
        R = BRW.insert(false);
        EXPECT_EQ(R.first, std::make_pair(State::Idle, State::Idle));
    }
    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::Idle, State::NotFilled));
    EXPECT_EQ(R.second, 1);
}

TEST(BitRateWindowTest, WithHoles) {
    size_t MinLength = 120, MaxLength = 240, IdleLength = 30;
    size_t SetBitsThreshold = 30;

    Result R;
    BitRateWindow BRW(MinLength, MaxLength, IdleLength, SetBitsThreshold);

    for (size_t Idx = 0; Idx != MaxLength; Idx++)
        R = BRW.insert(false);

    for (size_t Idx = 0; Idx != SetBitsThreshold / 3; Idx++)
        R = BRW.insert(true);
    for (size_t Idx = 0; Idx != SetBitsThreshold / 3; Idx++)
        R = BRW.insert(false);
    for (size_t Idx = 0; Idx != SetBitsThreshold / 3; Idx++)
        R = BRW.insert(true);
    for (size_t Idx = 0; Idx != SetBitsThreshold / 3; Idx++)
        R = BRW.insert(false);
    for (size_t Idx = 0; Idx != SetBitsThreshold / 3; Idx++)
        R = BRW.insert(true);

    EXPECT_EQ(R.first, std::make_pair(State::BelowThreshold, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);

    // The window's bit buffer contains:
    //      70 0s, 10 1s, 10 0s, 10 1s, 10 0s, 10 1s.
    // Where: 70 = MinLength - (5 / 3) * SetBitsThresholds, ie. we need
    // to add (70 + 1) more zeros to make the bit buffer go below the
    // threshold and then the window's length should be:
    //      70 + 50 + 70 = 190.

    BitRateWindow::Edge E;
    do {
        R = BRW.insert(false);
        E = R.first;
    } while (E.first != State::AboveThreshold || E.second != State::Idle);
    EXPECT_EQ(R.second, 2 * MinLength - (5 * SetBitsThreshold) / 3);
}

TEST(BitRateWindowTest, MinWindow) {
    size_t MinLength = 120, MaxLength = 240, IdleLength = 30;
    size_t SetBitsThreshold = 30;

    Result R;
    BitRateWindow BRW(MinLength, MaxLength, IdleLength, SetBitsThreshold);

    BRW.insert(true);
    BRW.insert(false);
    for (size_t Idx = 2; Idx != SetBitsThreshold; Idx++)
        BRW.insert(true);
    for (size_t Idx = SetBitsThreshold; Idx != MinLength - 1; Idx++)
        BRW.insert(false);

    R = BRW.insert(true);
    EXPECT_EQ(R.first, std::make_pair(State::NotFilled, State::AboveThreshold));
    EXPECT_EQ(R.second, MinLength);

    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::Idle));
}

TEST(BitRateWindowTest, MaxWindow) {
    size_t MinLength = 100, MaxLength = 200, IdleLength = 30;
    size_t SetBitsThreshold = 50;

    Result R;
    BitRateWindow BRW(MinLength, MaxLength, IdleLength, SetBitsThreshold);

    for (size_t Idx = 0; Idx != MaxLength; Idx++)
        R = BRW.insert(Idx % 2 == 0);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::AboveThreshold));
    EXPECT_EQ(R.second, MaxLength);

    R = BRW.insert(false);
    EXPECT_EQ(R.first, std::make_pair(State::AboveThreshold, State::Idle));
}
