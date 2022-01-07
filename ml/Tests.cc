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

// --------------------------------------------------------------------------------------------------------------------
// test ML API SQL query: SQL_SELECT_ANOMALY_RATE_INFO

/*The following test verifies the operation of the sal query that is included in the service of the ML_API_3
... the API is in charge of providing the peercentage of the time that each dimension has been anomalous
... within the given time range.
...The SQL query is obviously only for the part of the service when the given time range belongs to the past
... and its corresponding anomaly data is already saved in the database.
...For the other part of the service of the API service when the unsaved data is required, i.e. nearer the current time,
...a manual test is sought.*/
char *read_file_content_path(const char *filepath) {
    FILE *file = fopen(filepath, "r");
    if (file != nullptr) {
        fseek(file, 0L, SEEK_END);
        long file_size = ftell(file);
        rewind(file);
        char *text = (char*)malloc(file_size + 1);
        fread(text, 1, file_size, file);
        text[file_size] = '\0';
        fclose(file);
        return text;
    }
    else {
        return nullptr;
    }
}

int test_ml_callback(void *IgnoreMe, int argc, char **argv, char **TableField) {    
    (void) IgnoreMe;
    printf("Testing ML API SQL query; RESULTS:\n");  
    for (int i = 0; i < argc; i++) {
        printf("Testing ML API SQL query; %s = %s\n", TableField[i], argv[i] ? argv[i] : "NULL");
    }    
    return 0;
}

int run_ml_test_query(sqlite3 *db, const char *filepath) {
    char *err_msg = 0;
    char *sql = read_file_content_path(filepath);
    if(sql != NULL) {
        int rc = sqlite3_exec(db, sql, test_ml_callback, 0, &err_msg);    
        if (rc != SQLITE_OK ) {        
            fprintf(stderr, "Testing ML API SQL query; SQL error: %s\n", err_msg);        
            sqlite3_free(err_msg);      
            return 1;
        }
    }
    else {
        return 1;
    }
    return 0;
}

int test_ml_anomaly_info_api_sql(const std::string &AnomalyTestDBPath, const std::string &AnomalyTestDataPath, 
                                    const std::string &AnomalyTestQueryPath, const std::string &AnomalyTestCheckPath) {
    int retValue = 0;
    sqlite3 *db_ml_anomaly_info;

    
    int rc = sqlite3_open_v2(AnomalyTestDBPath.c_str(), &db_ml_anomaly_info, SQLITE_OPEN_READWRITE | SQLITE_OPEN_CREATE, NULL);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "Testing ML API SQL query; Cannot open database: %s\n", sqlite3_errmsg(db_ml_anomaly_info));
        sqlite3_close(db_ml_anomaly_info);        
        retValue = 1;
    }
    else {
        //Create and populate the test database from sql script
        if(run_ml_test_query(db_ml_anomaly_info, AnomalyTestDataPath.c_str()) == 0) {
            //execute the query under subject to this unit test
            if(run_ml_test_query(db_ml_anomaly_info, AnomalyTestQueryPath.c_str()) == 0) {
                //execute the query to check the results
                if(run_ml_test_query(db_ml_anomaly_info, AnomalyTestCheckPath.c_str()) != 0) {
                    fprintf(stderr, "Testing ML API SQL query; error reading query script at %s\n", AnomalyTestQueryPath.c_str());
                    retValue = 1;
                }        
            }
            else {
                fprintf(stderr, "Testing ML API SQL query; error reading check script at %s\n", AnomalyTestCheckPath.c_str());
                retValue = 1;
            }
        }
        else {
            fprintf(stderr, "Testing ML API SQL query; error reading data %s\n", AnomalyTestDataPath.c_str());
            retValue = 1;
        }  
    }
    sqlite3_close(db_ml_anomaly_info);    
    return retValue;
}

//Test 0: query a range where all source info have zero values of anomaly, so the average should be equal to zero
//... this is compared against a set result
TEST(AnomalyRateInfoSqlTest, AllDimZeroValues) {
    std::stringstream TestDBDirectory, TestDataDirectory, TestQueryDirectory, TestCheckDirectory;
    
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    std::string AnomalyTestDBPath = TestDBDirectory.str();
    
    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data_0.sql";
    std::string AnomalyTestDataPath = TestDataDirectory.str();
    
    TestQueryDirectory << netdata_configured_cache_dir << "/ml_test_query_0.sql";
    std::string AnomalyTestQueryPath = TestQueryDirectory.str();
    
    TestCheckDirectory << netdata_configured_cache_dir << "/ml_test_check_0.sql";
    std::string AnomalyTestCheckPath = TestCheckDirectory.str();
    
    EXPECT_EQ(test_ml_anomaly_info_api_sql(AnomalyTestDBPath, AnomalyTestDataPath, 
                                            AnomalyTestQueryPath, AnomalyTestCheckPath), 0);
}

//Test 1: query a range where all source info have equal values of anomaly, so the average should be equal to each value
//... this is compared against a set result
TEST(AnomalyRateInfoSqlTest, AllDimEqualValues) {
    std::stringstream TestDBDirectory, TestDataDirectory, TestQueryDirectory, TestCheckDirectory;
    
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    std::string AnomalyTestDBPath = TestDBDirectory.str();
    
    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data.sql";
    std::string AnomalyTestDataPath = TestDataDirectory.str();
    
    TestQueryDirectory << netdata_configured_cache_dir << "/ml_test_query_1.sql";
    std::string AnomalyTestQueryPath = TestQueryDirectory.str();
    
    TestCheckDirectory << netdata_configured_cache_dir << "/ml_test_check_1.sql";
    std::string AnomalyTestCheckPath = TestCheckDirectory.str();
    
    EXPECT_EQ(test_ml_anomaly_info_api_sql(AnomalyTestDBPath, AnomalyTestDataPath, 
                                            AnomalyTestQueryPath, AnomalyTestCheckPath), 0);
}

//Test 2: query a range where all info do not have equal values, so the average values should be different
//... and within the margin of the set results
TEST(AnomalyRateInfoSqlTest, DifferentDimValues) {
    std::stringstream TestDBDirectory, TestDataDirectory, TestQueryDirectory, TestCheckDirectory;
    
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    std::string AnomalyTestDBPath = TestDBDirectory.str();
    
    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data.sql";
    std::string AnomalyTestDataPath = TestDataDirectory.str();
    
    TestQueryDirectory << netdata_configured_cache_dir << "/ml_test_query_2.sql";
    std::string AnomalyTestQueryPath = TestQueryDirectory.str();
    
    TestCheckDirectory << netdata_configured_cache_dir << "/ml_test_check_2.sql";
    std::string AnomalyTestCheckPath = TestCheckDirectory.str();
    
    EXPECT_EQ(test_ml_anomaly_info_api_sql(AnomalyTestDBPath, AnomalyTestDataPath, 
                                            AnomalyTestQueryPath, AnomalyTestCheckPath), 0);
}

//Test 3: query portion of one range, so the average values should be equal to the values of the range
//... and within the margin of the set results
TEST(AnomalyRateInfoSqlTest, PortionOfOneRange) {
    std::stringstream TestDBDirectory, TestDataDirectory, TestQueryDirectory, TestCheckDirectory;
    
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    std::string AnomalyTestDBPath = TestDBDirectory.str();
    
    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data.sql";
    std::string AnomalyTestDataPath = TestDataDirectory.str();
    
    TestQueryDirectory << netdata_configured_cache_dir << "/ml_test_query_3.sql";
    std::string AnomalyTestQueryPath = TestQueryDirectory.str();
    
    TestCheckDirectory << netdata_configured_cache_dir << "/ml_test_check_3.sql";
    std::string AnomalyTestCheckPath = TestCheckDirectory.str();
    
    EXPECT_EQ(test_ml_anomaly_info_api_sql(AnomalyTestDBPath, AnomalyTestDataPath, 
                                            AnomalyTestQueryPath, AnomalyTestCheckPath), 0);
}

//Test 4: query a combination of two portions of two adjacent ranges, so the average values should be proportional
//... and within the margin of the set results
TEST(AnomalyRateInfoSqlTest, PortionsOfTwoRanges) {
    std::stringstream TestDBDirectory, TestDataDirectory, TestQueryDirectory, TestCheckDirectory;
    
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    std::string AnomalyTestDBPath = TestDBDirectory.str();
    
    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data.sql";
    std::string AnomalyTestDataPath = TestDataDirectory.str();
    
    TestQueryDirectory << netdata_configured_cache_dir << "/ml_test_query_4.sql";
    std::string AnomalyTestQueryPath = TestQueryDirectory.str();
    
    TestCheckDirectory << netdata_configured_cache_dir << "/ml_test_check_4.sql";
    std::string AnomalyTestCheckPath = TestCheckDirectory.str();
    
    EXPECT_EQ(test_ml_anomaly_info_api_sql(AnomalyTestDBPath, AnomalyTestDataPath, 
                                            AnomalyTestQueryPath, AnomalyTestCheckPath), 0);
}

//Test SimpleDB: query on a very simplified database of five records and two dimensions with only one none zero value
//... on one of the dimensions in one record, the result is compared against a set result
TEST(AnomalyRateInfoSqlTest, SimpleDimOneNoneZeroValue) {
    std::stringstream TestDBDirectory, TestDataDirectory, TestQueryDirectory, TestCheckDirectory;
    
    TestDBDirectory << netdata_configured_cache_dir << "/ml_test_anomaly_info.db";
    std::string AnomalyTestDBPath = TestDBDirectory.str();
    
    TestDataDirectory << netdata_configured_cache_dir << "/ml_test_data_simple.sql";
    std::string AnomalyTestDataPath = TestDataDirectory.str();
    
    TestQueryDirectory << netdata_configured_cache_dir << "/ml_test_query_simple.sql";
    std::string AnomalyTestQueryPath = TestQueryDirectory.str();
    
    TestCheckDirectory << netdata_configured_cache_dir << "/ml_test_check_simple.sql";
    std::string AnomalyTestCheckPath = TestCheckDirectory.str();
    
    EXPECT_EQ(test_ml_anomaly_info_api_sql(AnomalyTestDBPath, AnomalyTestDataPath, 
                                            AnomalyTestQueryPath, AnomalyTestCheckPath), 0);
}

