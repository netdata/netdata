#include <gtest/gtest.h>
#include "daemon/common.h"
#include "ml/json/single_include/nlohmann/json.hpp"

static void test_storage_number_loss(NETDATA_DOUBLE ND) {
    // Check precision loss of packing/unpacking
    {
        SN_FLAGS Flags = SN_DEFAULT_FLAGS;
        storage_number SN = pack_storage_number(ND, Flags);
        EXPECT_TRUE(does_storage_number_exist(SN));

        NETDATA_DOUBLE UnpackedND = unpack_storage_number(SN);

        NETDATA_DOUBLE AbsDiff = UnpackedND - ND;
        NETDATA_DOUBLE PctDiff = AbsDiff * 100.0 / ND;

        if (PctDiff < 0)
            PctDiff = - PctDiff;

        EXPECT_LT(PctDiff, ACCURACY_LOSS_ACCEPTED_PERCENT);
    }

    // check precision loss of custom formatting
    {
        char Buf[100];
        size_t Len = print_netdata_double(Buf, ND);
        EXPECT_EQ(strlen(Buf), Len);


        NETDATA_DOUBLE ParsedND = str2ndd(Buf, NULL);
        NETDATA_DOUBLE ParsedDiff = ND - ParsedND;
        NETDATA_DOUBLE PctParsedDiff = ParsedDiff * 100.0 / ND;

        if(PctParsedDiff < 0)
            PctParsedDiff = - PctParsedDiff;

        EXPECT_LT(PctParsedDiff, ACCURACY_LOSS_ACCEPTED_PERCENT);
    }
}

TEST(storage_number, precision_loss) {
    NETDATA_DOUBLE PosMinSN = unpack_storage_number(STORAGE_NUMBER_POSITIVE_MIN_RAW);
    NETDATA_DOUBLE NegMaxSN = unpack_storage_number(STORAGE_NUMBER_NEGATIVE_MAX_RAW);

    for (int g = -1; g <= 1 ; g++) {
        if(!g)
            continue;

        NETDATA_DOUBLE a = 0;
        for (int j = 0; j < 9 ;j++) {
            a += 0.0000001;

            NETDATA_DOUBLE c = a * g;
            for (int i = 0; i < 21 ;i++, c *= 10) {
                if (c > 0 && c < PosMinSN)
                    continue;
                if (c < 0 && c > NegMaxSN)
                    continue;

                test_storage_number_loss(c);
            }
        }
    }
}

TEST(storage_number, storage_number_exists) {
    storage_number sn = pack_storage_number(0.0, SN_DEFAULT_FLAGS);

    EXPECT_EQ(0.0, unpack_storage_number(sn));
}

TEST(storage_number, netdata_double_print) {
    using DoubleStringPair = std::pair<NETDATA_DOUBLE, const char *>;

    std::vector<DoubleStringPair> V = {
        { 0, "0" },
        { 0.0000001, "0.0000001" },
        { 0.00000009, "0.0000001" },
        { 0.000000001, "0" },
        { 99.99999999999999999, "100" },
        { -99.99999999999999999, "-100" },
        { 123.4567890123456789, "123.456789" },
        { 9999.9999999, "9999.9999999" },
        { -9999.9999999, "-9999.9999999" },
    };

    char Buf[50];
    for (const auto &P : V) {
        print_netdata_double(Buf, P.first);
        ASSERT_STREQ(Buf, P.second);
    }
}

TEST(storage_number, netdata_double_parse) {
    std::vector<std::string> Values = {
            "1.2345678", "-35.6", "0.00123", "23842384234234.2", ".1",
            "1.2e-10", "hello", "1wrong", "nan", "inf"
    };

    for (const std::string &S : Values) {
        char *EndPtrMine = nullptr;
        NETDATA_DOUBLE MineND = str2ndd(S.data(), &EndPtrMine);

        char *EndPtrSys = nullptr;
        NETDATA_DOUBLE SysND = strtondd(S.data(), &EndPtrSys);

        EXPECT_EQ(EndPtrMine, EndPtrSys);

        EXPECT_EQ(isnan(MineND), isnan(SysND));
        EXPECT_EQ(isinf(MineND), isinf(SysND));

        if (isnan(MineND) || isinf(MineND))
            continue;

        NETDATA_DOUBLE Diff= ABS(MineND - SysND);
        EXPECT_LT(Diff, 0.000001);
    }
}
