// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml/ml-private.h"
#include <gtest/gtest.h>

/*
 * The SamplesBuffer class implements the functionality of the following python
 * code:
 *      >> df = pd.DataFrame(data=samples)
 *      >> df = df.diff(diff_n).dropna()
 *      >> df = df.rolling(smooth_n).mean().dropna()
 *      >> df = pd.concat([df.shift(n) for n in range(lag_n + 1)], axis=1).dropna()
 *
 * Its correctness has been verified by automatically generating random
 * data frames in Python and comparing them with the correspondent preprocessed
 * SampleBuffers.
 *
 * The following tests are meant to catch unintended changes in the SamplesBuffer
 * implementation. For development purposes, one should compare changes against
 * the aforementioned python code.
*/

TEST(SamplesBufferTest, NS_8_NDPS_1_DN_1_SN_3_LN_1) {
    size_t NumSamples = 8, NumDimsPerSample = 1;
    size_t DiffN = 1, SmoothN = 3, LagN = 3;

    size_t N = NumSamples * NumDimsPerSample * (LagN + 1);
    CalculatedNumber *CNs = new CalculatedNumber[N]();

    CNs[0] = 0.7568336679490107;
    CNs[1] = 0.4814406581763254;
    CNs[2] = 0.40073555156221874;
    CNs[3] = 0.5973257298194408;
    CNs[4] = 0.5334727814345868;
    CNs[5] = 0.2632477193454843;
    CNs[6] = 0.2684839023122384;
    CNs[7] = 0.851332948637479;

    SamplesBuffer SB(CNs, NumSamples, NumDimsPerSample, DiffN, SmoothN, LagN);
    SB.preprocess();

    std::vector<Sample> Samples = SB.getPreprocessedSamples();
    EXPECT_EQ(Samples.size(), 2);

    Sample S0 = Samples[0];
    const CalculatedNumber *S0_CNs = S0.getCalculatedNumbers();
    Sample S1 = Samples[1];
    const CalculatedNumber *S1_CNs = S1.getCalculatedNumbers();

    EXPECT_NEAR(S0_CNs[0], -0.109614, 0.001);
    EXPECT_NEAR(S0_CNs[1], -0.0458293, 0.001);
    EXPECT_NEAR(S0_CNs[2], 0.017344, 0.001);
    EXPECT_NEAR(S0_CNs[3], -0.0531693, 0.001);

    EXPECT_NEAR(S1_CNs[0], 0.105953, 0.001);
    EXPECT_NEAR(S1_CNs[1], -0.109614, 0.001);
    EXPECT_NEAR(S1_CNs[2], -0.0458293, 0.001);
    EXPECT_NEAR(S1_CNs[3], 0.017344, 0.001);

    delete[] CNs;
}

TEST(SamplesBufferTest, NS_8_NDPS_1_DN_2_SN_3_LN_2) {
    size_t NumSamples = 8, NumDimsPerSample = 1;
    size_t DiffN = 2, SmoothN = 3, LagN = 2;

    size_t N = NumSamples * NumDimsPerSample * (LagN + 1);
    CalculatedNumber *CNs = new CalculatedNumber[N]();

    CNs[0] = 0.20511885291342846;
    CNs[1] = 0.13151717360306558;
    CNs[2] = 0.6017085062423134;
    CNs[3] = 0.46256882933941545;
    CNs[4] = 0.7887758447877941;
    CNs[5] = 0.9237989080034406;
    CNs[6] = 0.15552559051428083;
    CNs[7] = 0.6309750314597955;

    SamplesBuffer SB(CNs, NumSamples, NumDimsPerSample, DiffN, SmoothN, LagN);
    SB.preprocess();

    std::vector<Sample> Samples = SB.getPreprocessedSamples();
    EXPECT_EQ(Samples.size(), 2);

    Sample S0 = Samples[0];
    const CalculatedNumber *S0_CNs = S0.getCalculatedNumbers();
    Sample S1 = Samples[1];
    const CalculatedNumber *S1_CNs = S1.getCalculatedNumbers();

    EXPECT_NEAR(S0_CNs[0], 0.005016, 0.001);
    EXPECT_NEAR(S0_CNs[1], 0.326450, 0.001);
    EXPECT_NEAR(S0_CNs[2], 0.304903, 0.001);

    EXPECT_NEAR(S1_CNs[0], -0.154948, 0.001);
    EXPECT_NEAR(S1_CNs[1], 0.005016, 0.001);
    EXPECT_NEAR(S1_CNs[2], 0.326450, 0.001);

    delete[] CNs;
}

TEST(SamplesBufferTest, NS_8_NDPS_3_DN_2_SN_4_LN_1) {
    size_t NumSamples = 8, NumDimsPerSample = 3;
    size_t DiffN = 2, SmoothN = 4, LagN = 1;

    size_t N = NumSamples * NumDimsPerSample * (LagN + 1);
    CalculatedNumber *CNs = new CalculatedNumber[N]();

    CNs[0] = 0.34310900399667765; CNs[1] = 0.14694315994488194; CNs[2] = 0.8246677800938796;
    CNs[3] = 0.48249504592307835; CNs[4] = 0.23241087965531182; CNs[5] = 0.9595348555892567;
    CNs[6] = 0.44281094035598334; CNs[7] = 0.5143142171362715; CNs[8] = 0.06391303014242555;
    CNs[9] = 0.7460491027783901; CNs[10] = 0.43887217459032923; CNs[11] = 0.2814395025355999;
    CNs[12] = 0.9231114281214198; CNs[13] = 0.326882401786898; CNs[14] = 0.26747939220376216;
    CNs[15] = 0.7787571209969636; CNs[16] =0.5851700001235088; CNs[17] = 0.34410728945321567;
    CNs[18] = 0.9394494507088997; CNs[19] =0.17567223681734334; CNs[20] = 0.42732886195446984;
    CNs[21] = 0.9460522396152958; CNs[22] =0.23462747016780894; CNs[23] = 0.35983249900892145;

    SamplesBuffer SB(CNs, NumSamples, NumDimsPerSample, DiffN, SmoothN, LagN);
    SB.preprocess();

    std::vector<Sample> Samples = SB.getPreprocessedSamples();
    EXPECT_EQ(Samples.size(), 2);

    Sample S0 = Samples[0];
    const CalculatedNumber *S0_CNs = S0.getCalculatedNumbers();
    Sample S1 = Samples[1];
    const CalculatedNumber *S1_CNs = S1.getCalculatedNumbers();

    EXPECT_NEAR(S0_CNs[0], 0.198225, 0.001);
    EXPECT_NEAR(S0_CNs[1], 0.003529, 0.001);
    EXPECT_NEAR(S0_CNs[2], -0.063003, 0.001);
    EXPECT_NEAR(S0_CNs[3], 0.219066, 0.001);
    EXPECT_NEAR(S0_CNs[4], 0.133175, 0.001);
    EXPECT_NEAR(S0_CNs[5], -0.293154, 0.001);

    EXPECT_NEAR(S1_CNs[0], 0.174160, 0.001);
    EXPECT_NEAR(S1_CNs[1], -0.135722, 0.001);
    EXPECT_NEAR(S1_CNs[2], 0.110452, 0.001);
    EXPECT_NEAR(S1_CNs[3], 0.198225, 0.001);
    EXPECT_NEAR(S1_CNs[4], 0.003529, 0.001);
    EXPECT_NEAR(S1_CNs[5], -0.063003, 0.001);

    delete[] CNs;
}
