// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_DATABASE_H
#define ML_DATABASE_H

#include "Dimension.h"
#include "ml-private.h"

#include "json/single_include/nlohmann/json.hpp"

namespace ml {

class Statement {
public:
    using RowCallback = std::function<void(sqlite3_stmt *Stmt)>;

public:
    Statement(const char *RawStmt) : RawStmt(RawStmt), ParsedStmt(nullptr) {}

    template<typename ...ArgTypes>
    bool exec(sqlite3 *Conn, RowCallback RowCb, ArgTypes ...Args) {
        if (!prepare(Conn))
            return false;

        switch (bind(1, Args...)) {
        case 0:
            return false;
        case sizeof...(Args):
            break;
        default:
            return resetAndClear(false);
        }

        while (true) {
            switch (int RC = sqlite3_step(ParsedStmt)) {
            case SQLITE_BUSY: case SQLITE_LOCKED:
                usleep(SQLITE_INSERT_DELAY * USEC_PER_MS);
                continue;
            case SQLITE_ROW:
                RowCb(ParsedStmt);
                continue;
            case SQLITE_DONE:
                return resetAndClear(true);
            default:
                error("Stepping through '%s' returned rc=%d", RawStmt, RC);
                return resetAndClear(false);
            }
        }
    }

    ~Statement() {
        if (!ParsedStmt)
            return;

        int RC = sqlite3_finalize(ParsedStmt);
        if (RC != SQLITE_OK)
            error("Could not properly finalize statement (rc=%d)", RC);
    }

private:
    bool prepare(sqlite3 *Conn);

    bool bindValue(size_t Pos, const int Value);
    bool bindValue(size_t Pos, const std::string &Value);

    template<typename ArgType, typename ...ArgTypes>
    size_t bind(size_t Pos, ArgType T) {
        return bindValue(Pos, T);
    }

    template<typename ArgType, typename ...ArgTypes>
    size_t bind(size_t Pos, ArgType T, ArgTypes ...Args) {
        return bindValue(Pos, T) + bind(Pos + 1, Args...);
    }

    bool resetAndClear(bool Ret);

private:
    const char *RawStmt;
    sqlite3_stmt *ParsedStmt;
};

class Database {
private:
    static const char *SQL_CREATE_ANOMALIES_TABLE;
    static const char *SQL_INSERT_ANOMALY;
    static const char *SQL_SELECT_ANOMALY;
    static const char *SQL_SELECT_ANOMALY_EVENTS;
    
    static const char *SQL_CREATE_ANOMALY_RATE_INFO_TABLE;
    static const char *SQL_INSERT_BULK_ANOMALY_RATE_INFO;
    static const char *SQL_SELECT_ANOMALY_RATE_INFO;
    static const char *SQL_SELECT_ANOMALY_RATE_INFO_RANGE;
    static const char *SQL_REMOVE_OLD_ANOMALY_RATE_INFO;

public:
    Database(const std::string &Path);

    ~Database();

    template<typename ...ArgTypes>
    bool insertAnomaly(ArgTypes... Args) {
        Statement::RowCallback RowCb = [](sqlite3_stmt *Stmt) { (void) Stmt; };
        return InsertAnomalyStmt.exec(Conn, RowCb, Args...);
    }

    template<typename ...ArgTypes>
    bool getAnomalyInfo(nlohmann::json &Json, ArgTypes&&... Args) {
        Statement::RowCallback RowCb = [&](sqlite3_stmt *Stmt) {
            const char *Text = static_cast<const char *>(sqlite3_column_blob(Stmt, 0));
            Json = nlohmann::json::parse(Text);
        };
        return GetAnomalyInfoStmt.exec(Conn, RowCb, Args...);
    }

    template<typename ...ArgTypes>
    bool getAnomaliesInRange(std::vector<std::pair<time_t, time_t>> &V, ArgTypes&&... Args) {
        Statement::RowCallback RowCb = [&](sqlite3_stmt *Stmt) {
            V.push_back({
                sqlite3_column_int64(Stmt, 0),
                sqlite3_column_int64(Stmt, 1)
            });
        };
        return GetAnomaliesInRangeStmt.exec(Conn, RowCb, Args...);
    }

    template<typename ...ArgTypes>
    bool insertBulkAnomalyRateInfo(ArgTypes... Args) {
        Statement::RowCallback RowCb = [](sqlite3_stmt *Stmt) { (void) Stmt; };
        return InsertBulkAnomalyRateInfoStmt.exec(Conn, RowCb, Args...);
    }

    template<typename ...ArgTypes>
    bool getAnomalyRateInfoInRange(std::vector<std::pair<std::string, double>> &V, ArgTypes&&... Args) {
        Statement::RowCallback RowCb = [&](sqlite3_stmt *Stmt) {
            V.push_back({
                reinterpret_cast<const char*>(sqlite3_column_text(Stmt, 0)),
                sqlite3_column_double(Stmt, 1)
            });
        };
        return GetAnomalyRateInfoInRangeStmt.exec(Conn, RowCb, Args...);
    }

    template<typename ...ArgTypes>
    bool getTheLastSavedAnomalyInfoRange(std::pair<int, int> &P, ArgTypes&&... Args) {
        Statement::RowCallback RowCb = [&](sqlite3_stmt *Stmt) {
            P.first = sqlite3_column_int64(Stmt, 0);
            P.second = sqlite3_column_int64(Stmt, 1);
        };
        return GetTheLastSavedAnomalyInfoRangeStmt.exec(Conn, RowCb, Args...);
    }

    template<typename ...ArgTypes>
    void removeOldAnomalyRateInfo(ArgTypes&&... Args) {
        Statement::RowCallback RowCb = [&](sqlite3_stmt *Stmt) { (void) Stmt; };
        RemoveOldAnomalyRateInfoStmt.exec(Conn, RowCb, Args...);
    }

private:
    sqlite3 *Conn;

    Statement InsertAnomalyStmt{SQL_INSERT_ANOMALY};
    Statement GetAnomalyInfoStmt{SQL_SELECT_ANOMALY};
    Statement GetAnomaliesInRangeStmt{SQL_SELECT_ANOMALY_EVENTS};
    
    Statement InsertBulkAnomalyRateInfoStmt{SQL_INSERT_BULK_ANOMALY_RATE_INFO};
    Statement GetAnomalyRateInfoInRangeStmt{SQL_SELECT_ANOMALY_RATE_INFO};
    Statement GetTheLastSavedAnomalyInfoRangeStmt{SQL_SELECT_ANOMALY_RATE_INFO_RANGE};
    Statement RemoveOldAnomalyRateInfoStmt{SQL_REMOVE_OLD_ANOMALY_RATE_INFO};
};

}

#endif /* ML_DATABASE_H */
