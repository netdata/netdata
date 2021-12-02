// SPDX-License-Identifier: GPL-3.0-or-later

#include "Config.h"
#include "Dimension.h"
#include "Host.h"

using namespace ml;

/*
 * Assumptions:
 *  1) hosts outlive their sets, and sets outlive their dimensions,
 *  2) dimensions always have a set that has a host.
 */

void ml_init(void) {
    Cfg.readMLConfig();
}

void ml_new_host(RRDHOST *RH) {
    if (!Cfg.EnableAnomalyDetection)
        return;

    if (simple_pattern_matches(Cfg.SP_HostsToSkip, RH->hostname))
        return;

    Host *H = new Host(RH);
    RH->ml_host = static_cast<ml_host_t>(H);

    H->startAnomalyDetectionThreads();
}

void ml_delete_host(RRDHOST *RH) {
    Host *H = static_cast<Host *>(RH->ml_host);
    if (!H)
        return;

    H->stopAnomalyDetectionThreads();

    delete H;
    RH->ml_host = nullptr;
}

void ml_new_dimension(RRDDIM *RD) {
    RRDSET *RS = RD->rrdset;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    if (!H)
        return;

    if (static_cast<unsigned>(RD->update_every) != H->updateEvery())
        return;

    if (simple_pattern_matches(Cfg.SP_ChartsToSkip, RS->name))
        return;

    Dimension *D = new Dimension(RD);
    RD->state->ml_dimension = static_cast<ml_dimension_t>(D);
    H->addDimension(D);
}

void ml_delete_dimension(RRDDIM *RD) {
    Dimension *D = static_cast<Dimension *>(RD->state->ml_dimension);
    if (!D)
        return;

    Host *H = static_cast<Host *>(RD->rrdset->rrdhost->ml_host);
    H->removeDimension(D);

    RD->state->ml_dimension = nullptr;
}

char *ml_get_host_info(RRDHOST *RH) {
    nlohmann::json ConfigJson;

    if (RH && RH->ml_host) {
        Host *H = static_cast<Host *>(RH->ml_host);
        H->getConfigAsJson(ConfigJson);
        H->getDetectionInfoAsJson(ConfigJson);
    } else {
        ConfigJson["enabled"] = false;
    }

    return strdup(ConfigJson.dump(2, '\t').c_str());
}

bool ml_is_anomalous(RRDDIM *RD, double Value, bool Exists) {
    Dimension *D = static_cast<Dimension *>(RD->state->ml_dimension);
    if (!D)
        return false;

    D->addValue(Value, Exists);
    bool Result = D->predict().second;
    return Result;
}

char *ml_get_anomaly_events(RRDHOST *RH, const char *AnomalyDetectorName,
                            int AnomalyDetectorVersion, time_t After, time_t Before) {
    if (!RH || !RH->ml_host) {
        error("No host");
        return nullptr;
    }

    Host *H = static_cast<Host *>(RH->ml_host);
    std::vector<std::pair<time_t, time_t>> TimeRanges;

    bool Res = H->getAnomaliesInRange(TimeRanges, AnomalyDetectorName,
                                                  AnomalyDetectorVersion,
                                                  H->getUUID(),
                                                  After, Before);
    if (!Res) {
        error("DB result is empty");
        return nullptr;
    }

    nlohmann::json Json = TimeRanges;
    return strdup(Json.dump(4).c_str());
}

char *ml_get_anomaly_event_info(RRDHOST *RH, const char *AnomalyDetectorName,
                                int AnomalyDetectorVersion, time_t After, time_t Before) {
    if (!RH || !RH->ml_host) {
        error("No host");
        return nullptr;
    }

    Host *H = static_cast<Host *>(RH->ml_host);

    nlohmann::json Json;
    bool Res = H->getAnomalyInfo(Json, AnomalyDetectorName,
                                    AnomalyDetectorVersion,
                                    H->getUUID(),
                                    After, Before);
    if (!Res) {
        error("DB result is empty");
        return nullptr;
    }

    return strdup(Json.dump(4, '\t').c_str());
}

char *ml_get_anomaly_rate_info(RRDHOST *RH, time_t After, time_t Before) {
    if (!RH || !RH->ml_host) {
        error("No host");
        return nullptr;
    }

    Host *H = static_cast<Host *>(RH->ml_host);
    std::vector<std::pair<std::string, double>> DimAndAnomalyRate;
    if(Before > After) {
        if(Before <= H->getLastSavedBefore()) {
            //Only information from saved data is inquired
            bool Res = H->getAnomalyRateInfoInRange(DimAndAnomalyRate, H->getUUID(),
                                                        After, Before);
            if (!Res) {
                error("DB result is empty");
                return nullptr;
            }
        }
        else {
            //Information from unsaved data is also inquired
            if(Before > now_realtime_sec()) { 
                Before = now_realtime_sec();
            }
            if(After >= H->getLastSavedBefore()) {
                //Only the information from unsaved data is inquired
                H->getAnomalyRateInfoCurrentRange(DimAndAnomalyRate, After, Before);
            }
            else {
                //Mix information from saved and unsaved data is inquired
                H->getAnomalyRateInfoMixedRange(DimAndAnomalyRate, H->getUUID(),
                                                        After, Before);
            }
        }
    }
    else
    {
        error("Incorrect time range; Before time tag is not larger than After time tag!");
        return nullptr;
    }

    nlohmann::json Json = DimAndAnomalyRate;
    return strdup(Json.dump(4).c_str());
}

#if defined(ENABLE_ML_TESTS)

#include "gtest/gtest.h"

int test_ml(int argc, char *argv[]) {
    (void) argc;
    (void) argv;

    test_ml_anomaly_info_api_sql();

    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}

// --------------------------------------------------------------------------------------------------------------------
// test ML API SQL query: SQL_SELECT_ANOMALY_RATE_INFO
char *read_file_content_path(char const *filepath) {
    FILE *file = fopen(filepath, "r");
    if (file != NULL) {
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
        return "NULL!";
    }
}

int test_ml_callback(void *IgnoreMe, int argc, char **argv, char **TableField) {    
    IgnoreMe = 0;
    printf("Testing ML API SQL query; RESULTS:\n");  
    for (int i = 0; i < argc; i++) {
        printf("Testing ML API SQL query; %s = %s\n", TableField[i], argv[i] ? argv[i] : "NULL");
    }    
    return 0;
}

int run_ml_test_query(sqlite3 *db, char const *filepath) {
    char *err_msg = 0;
    char *sql = read_file_content_path(filepath);
    if(sql != "NULL!") {
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

/*The following test verifies the operation of the sal query that is included in the service of the ML_API_3
... the API is in charge of providing the peercentage of the time that each dimension has been anomalous
... within the given time range.
...The SQL query is obviously only for the part of the service when the given time range belongs to the past
... and its corresponding anomaly data is already saved in the database.
...For the other part of the service of the API service when the unsaved data is required, i.e. nearer the current time,
...a manual test is sought.
In order to run the following test, please first, change the file path of the db file and the sql script files
...to your home/???/opt/... directory where all your test .sql files should reside. 
...Otherwise, a common variable from some config file should be used to replace the directory*/
int test_ml_anomaly_info_api_sql(void) {
    int retValue = 0;
    fprintf(stderr, "Testing ML API SQL query; SQL_SELECT_ANOMALY_RATE_INFO\n");
    sqlite3 *db_ml_anomaly_info;
        
    int rc = sqlite3_open("/home/siamak/opt/netdata/var/cache/netdata/ml_test_anomaly_info.db", &db_ml_anomaly_info);
    if (rc != SQLITE_OK) {
        fprintf(stderr, "Testing ML API SQL query; Cannot open database: %s\n", sqlite3_errmsg(db_ml_anomaly_info));
        sqlite3_close(db_ml_anomaly_info);        
        retValue = 1;
    }
    else {
        //Create and populate the test database from sql script
        if(run_ml_test_query(db_ml_anomaly_info, "/home/siamak/opt/netdata/var/cache/netdata/ml_test_data.sql") == 0) {
            //Test 1: query a range where all source info have equal values of anomaly, so the average should be equal to each value
            //... this is compared against a set result
            fprintf(stderr, "Testing ML API SQL query; SQL_SELECT_ANOMALY_RATE_INFO; Test 1:\n");
            //execute the query under subject to this unit test
            if(run_ml_test_query(db_ml_anomaly_info, "/home/siamak/opt/netdata/var/cache/netdata/ml_test_query_1.sql") == 0) {
                //execute the query to check the results
                if(run_ml_test_query(db_ml_anomaly_info, "/home/siamak/opt/netdata/var/cache/netdata/ml_test_check_1.sql") == 0) {
            
                }
                else {
                    fprintf(stderr, "Testing ML API SQL query; error reading sql file ml_test_check_1.sql\n");
                    retValue = 1;
                }        
            }
            else {
                fprintf(stderr, "Testing ML API SQL query; error reading sql file ml_test_query_1.sql\n");
                retValue = 1;
            }
            //Test 2: query a range where all info do not have equal values, so the average values should be different
            //... and within the margin of the set results
            fprintf(stderr, "Testing ML API SQL query; SQL_SELECT_ANOMALY_RATE_INFO; Test 2:\n");
            //execute the query under subject to this unit test
            if(run_ml_test_query(db_ml_anomaly_info, "/home/siamak/opt/netdata/var/cache/netdata/ml_test_query_2.sql") == 0) {
                //execute the query to check the results
                if(run_ml_test_query(db_ml_anomaly_info, "/home/siamak/opt/netdata/var/cache/netdata/ml_test_check_2.sql") == 0) {
            
                }
                else {
                    fprintf(stderr, "Testing ML API SQL query; error reading sql file ml_test_check_2.sql\n");
                    retValue = 1;
                }        
            }
            else {
                fprintf(stderr, "Testing ML API SQL query; error reading sql file ml_test_query_2.sql\n");
                retValue = 1;
            }

        }
        else {
            fprintf(stderr, "Testing ML API SQL query; error reading sql file ml_test_data.sql\n");
            retValue = 1;
        }  
    }
    sqlite3_close(db_ml_anomaly_info);    
    return retValue;
}


#endif // ENABLE_ML_TESTS
