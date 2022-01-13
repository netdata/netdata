// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef SAMPLES_BUFFER_H
#define SAMPLES_BUFFER_H

#include <iostream>
#include <vector>

#include <cassert>
#include <cstdlib>
#include <cstring>

#include <dlib/matrix.h>

typedef double CalculatedNumber;
typedef dlib::matrix<CalculatedNumber, 0, 1> DSample;

class Sample {
public:
    Sample(CalculatedNumber *Buf, size_t N) : CNs(Buf), NumDims(N) {}

    void initDSample(DSample &DS) const {
        for (size_t Idx = 0; Idx != NumDims; Idx++) {
            DS(Idx) = std::abs(CNs[Idx]);
        }
    }

    void add(const Sample &RHS) const {
        assert(NumDims == RHS.NumDims);

        for (size_t Idx = 0; Idx != NumDims; Idx++)
            CNs[Idx] += RHS.CNs[Idx];
    };

    void diff(const Sample &RHS) const {
        assert(NumDims == RHS.NumDims);

        for (size_t Idx = 0; Idx != NumDims; Idx++)
            CNs[Idx] -= RHS.CNs[Idx];
    };

    void copy(const Sample &RHS) const {
        assert(NumDims == RHS.NumDims);

        std::memcpy(CNs, RHS.CNs, NumDims * sizeof(CalculatedNumber));
    }

    void scale(CalculatedNumber Factor) {
        for (size_t Idx = 0; Idx != NumDims; Idx++)
            CNs[Idx] *= Factor;
    }

    void lag(const Sample &S, size_t LagN) {
        size_t N = S.NumDims;

        for (size_t Idx = 0; Idx != (LagN + 1); Idx++) {
            Sample Src(S.CNs - (Idx * N), N);
            Sample Dst(CNs + (Idx * N), N);
            Dst.copy(Src);
        }
    }

    const CalculatedNumber *getCalculatedNumbers() const {
        return CNs;
    };

    void print(std::ostream &OS) const;

private:
    CalculatedNumber *CNs;
    size_t NumDims;
};

inline std::ostream& operator<<(std::ostream &OS, const Sample &S) {
    S.print(OS);
    return OS;
}

class SamplesBuffer {
public:
    SamplesBuffer(CalculatedNumber *CNs,
                  size_t NumSamples, size_t NumDimsPerSample,
                  size_t DiffN = 1, size_t SmoothN = 3, size_t LagN = 3) :
        CNs(CNs), NumSamples(NumSamples), NumDimsPerSample(NumDimsPerSample),
        DiffN(DiffN), SmoothN(SmoothN), LagN(LagN),
        BytesPerSample(NumDimsPerSample * sizeof(CalculatedNumber)),
        Preprocessed(false) {};

    std::vector<DSample> preprocess();
    std::vector<Sample> getPreprocessedSamples() const;

    size_t capacity() const { return NumSamples; }
    void print(std::ostream &OS) const;

private:
    size_t getSampleOffset(size_t Index) const {
        assert(Index < NumSamples);
        return Index * NumDimsPerSample;
    }

    size_t getPreprocessedSampleOffset(size_t Index) const {
        assert(Index < NumSamples);
        return getSampleOffset(Index) * (LagN + 1);
    }

    void setSample(size_t Index, const Sample &S) const {
        size_t Offset = getSampleOffset(Index);
        std::memcpy(&CNs[Offset], S.getCalculatedNumbers(), BytesPerSample);
    }

    const Sample getSample(size_t Index) const {
        size_t Offset = getSampleOffset(Index);
        return Sample(&CNs[Offset], NumDimsPerSample);
    };

    const Sample getPreprocessedSample(size_t Index) const {
        size_t Offset = getPreprocessedSampleOffset(Index);
        return Sample(&CNs[Offset], NumDimsPerSample * (LagN + 1));
    };

    void diffSamples();
    void smoothSamples();
    void lagSamples();

private:
    CalculatedNumber *CNs;
    size_t NumSamples;
    size_t NumDimsPerSample;
    size_t DiffN;
    size_t SmoothN;
    size_t LagN;
    size_t BytesPerSample;
    bool Preprocessed;
};

inline std::ostream& operator<<(std::ostream& OS, const SamplesBuffer &SB) {
    SB.print(OS);
    return OS;
}

#endif /* SAMPLES_BUFFER_H */
