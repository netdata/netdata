// SPDX-License-Identifier: GPL-3.0-or-later
//
#include "SamplesBuffer.h"

#include <fstream>
#include <sstream>
#include <string>

void Sample::print(std::ostream &OS) const {
    for (size_t Idx = 0; Idx != NumDims - 1; Idx++)
        OS << CNs[Idx] << ", ";

    OS << CNs[NumDims - 1];
}

void SamplesBuffer::print(std::ostream &OS) const {
    for (size_t Idx = Preprocessed ? (DiffN + (SmoothN - 1) + (LagN)) : 0;
         Idx != NumSamples; Idx++) {
        Sample S = Preprocessed ? getPreprocessedSample(Idx) : getSample(Idx);
        OS << S << std::endl;
    }
}

std::vector<Sample> SamplesBuffer::getPreprocessedSamples() const {
    std::vector<Sample> V;

    for (size_t Idx = Preprocessed ? (DiffN + (SmoothN - 1) + (LagN)) : 0;
         Idx != NumSamples; Idx++) {
        Sample S = Preprocessed ? getPreprocessedSample(Idx) : getSample(Idx);
        V.push_back(S);
    }

    return V;
}

void SamplesBuffer::diffSamples() {
    // Panda's DataFrame default behaviour is to subtract each element from
    // itself. For us `DiffN = 0` means "disable diff-ing" when preprocessing
    // the samples buffer. This deviation will make it easier for us to test
    // the KMeans implementation.
    if (DiffN == 0)
        return;

    for (size_t Idx = 0; Idx != (NumSamples - DiffN); Idx++) {
        size_t High = (NumSamples - 1) - Idx;
        size_t Low = High - DiffN;

        Sample LHS = getSample(High);
        Sample RHS = getSample(Low);

        LHS.diff(RHS);
    }
}

void SamplesBuffer::smoothSamples() {
    // Holds the mean value of each window
    CalculatedNumber *AccCNs = new CalculatedNumber[NumDimsPerSample]();
    Sample Acc(AccCNs, NumDimsPerSample);

    // Used to avoid clobbering the accumulator when moving the window
    CalculatedNumber *TmpCNs = new CalculatedNumber[NumDimsPerSample]();
    Sample Tmp(TmpCNs, NumDimsPerSample);

    CalculatedNumber Factor = (CalculatedNumber) 1 / SmoothN;

    // Calculate the value of the 1st window
    for (size_t Idx = 0; Idx != std::min(SmoothN, NumSamples); Idx++) {
        Tmp.add(getSample(NumSamples - (Idx + 1)));
    }

    Acc.add(Tmp);
    Acc.scale(Factor);

    // Move the window and update the samples
    for (size_t Idx = NumSamples; Idx != (DiffN + SmoothN - 1); Idx--) {
        Sample S = getSample(Idx - 1);

        // Tmp <- Next window (if any)
        if (Idx >= (SmoothN + 1)) {
            Tmp.diff(S);
            Tmp.add(getSample(Idx - (SmoothN + 1)));
        }

        // S <- Acc
        S.copy(Acc);

        // Acc <- Tmp
        Acc.copy(Tmp);
        Acc.scale(Factor);
    }

    delete[] AccCNs;
    delete[] TmpCNs;
}

void SamplesBuffer::lagSamples() {
    if (LagN == 0)
        return;

    for (size_t Idx = NumSamples; Idx != LagN; Idx--) {
        Sample PS = getPreprocessedSample(Idx - 1);
        PS.lag(getSample(Idx - 1), LagN);
    }
}

std::vector<DSample> SamplesBuffer::preprocess() {
    assert(Preprocessed == false);

    std::vector<DSample> DSamples;
    size_t OutN = NumSamples;

    // Diff
    if (DiffN >= OutN)
        return DSamples;
    OutN -= DiffN;
    diffSamples();

    // Smooth
    if (SmoothN == 0 || SmoothN > OutN)
        return DSamples;
    OutN -= (SmoothN - 1);
    smoothSamples();

    // Lag
    if (LagN >= OutN)
        return DSamples;
    OutN -= LagN;
    lagSamples();

    DSamples.reserve(OutN);
    Preprocessed = true;

    uint32_t MaxMT = std::numeric_limits<uint32_t>::max();
    uint32_t CutOff = static_cast<double>(MaxMT) * SamplingRatio;

    for (size_t Idx = NumSamples - OutN; Idx != NumSamples; Idx++) {
        if (RandNums[Idx] > CutOff)
            continue;

        DSample DS;
        DS.set_size(NumDimsPerSample * (LagN + 1));

        const Sample PS = getPreprocessedSample(Idx);
        PS.initDSample(DS);

        DSamples.push_back(DS);
    }

    return DSamples;
}
