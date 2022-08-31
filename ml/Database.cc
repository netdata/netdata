// SPDX-License-Identifier: GPL-3.0-or-later

#include "Database.h"

const char *ml::Database::SQL_CREATE_ANOMALIES_TABLE =
    "CREATE TABLE IF NOT EXISTS anomaly_events( "
    "     anomaly_detector_name text NOT NULL, "
    "     anomaly_detector_version int NOT NULL, "
    "     host_id text NOT NULL, "
    "     after int NOT NULL, "
    "     before int NOT NULL, "
    "     anomaly_event_info text, "
    "     PRIMARY KEY( "
    "         anomaly_detector_name, anomaly_detector_version, "
    "         host_id, after, before "
    "     ) "
    ");";

const char *ml::Database::SQL_INSERT_ANOMALY =
    "INSERT INTO anomaly_events( "
    "     anomaly_detector_name, anomaly_detector_version, "
    "     host_id, after, before, anomaly_event_info) "
    "VALUES (?1, ?2, ?3, ?4, ?5, ?6);";

const char *ml::Database::SQL_SELECT_ANOMALY =
    "SELECT anomaly_event_info FROM anomaly_events WHERE"
    "   anomaly_detector_name == ?1 AND"
    "   anomaly_detector_version == ?2 AND"
    "   host_id == ?3 AND"
    "   after == ?4 AND"
    "   before == ?5;";

const char *ml::Database::SQL_SELECT_ANOMALY_EVENTS =
    "SELECT after, before FROM anomaly_events WHERE"
    "   anomaly_detector_name == ?1 AND"
    "   anomaly_detector_version == ?2 AND"
    "   host_id == ?3 AND"
    "   after >= ?4 AND"
    "   before <= ?5;";

using namespace ml;

bool Statement::prepare(sqlite3 *Conn) {
    if (!Conn)
        return false;

    if (ParsedStmt)
        return true;

    int RC = sqlite3_prepare_v2(Conn, RawStmt, -1, &ParsedStmt, nullptr);
    if (RC == SQLITE_OK)
        return true;

    std::string Msg = "Statement \"%s\" preparation failed due to \"%s\"";
    error(Msg.c_str(), RawStmt, sqlite3_errstr(RC));

    return false;
}

bool Statement::bindValue(size_t Pos, const std::string &Value) {
    int RC = sqlite3_bind_text(ParsedStmt, Pos, Value.c_str(), -1, SQLITE_TRANSIENT);
    if (RC == SQLITE_OK)
        return true;

    error("Failed to bind text '%s' (pos = %zu) in statement '%s'.", Value.c_str(), Pos, RawStmt);
    return false;
}

bool Statement::bindValue(size_t Pos, const int Value) {
    int RC = sqlite3_bind_int(ParsedStmt, Pos, Value);
    if (RC == SQLITE_OK)
        return true;

    error("Failed to bind integer %d (pos = %zu) in statement '%s'.", Value, Pos, RawStmt);
    return false;
}

bool Statement::resetAndClear(bool Ret) {
    int RC = sqlite3_reset(ParsedStmt);
    if (RC != SQLITE_OK) {
        error("Could not reset statement: '%s'", RawStmt);
        return false;
    }

    RC = sqlite3_clear_bindings(ParsedStmt);
    if (RC != SQLITE_OK) {
        error("Could not clear bindings in statement: '%s'", RawStmt);
        return false;
    }

    return Ret;
}

Database::Database(const std::string &Path) {
    // Get sqlite3 connection handle.
    int RC = sqlite3_open(Path.c_str(), &Conn);
    if (RC != SQLITE_OK) {
        std::string Msg = "Failed to initialize ML DB at %s, due to \"%s\"";
        error(Msg.c_str(), Path.c_str(), sqlite3_errstr(RC));

        sqlite3_close(Conn);
        Conn = nullptr;
        return;
    }

    // Create anomaly events table if it does not exist.
    char *ErrMsg;
    RC = sqlite3_exec_monitored(Conn, SQL_CREATE_ANOMALIES_TABLE, nullptr, nullptr, &ErrMsg);
    if (RC == SQLITE_OK)
        return;

    error("SQLite error during database initialization, rc = %d (%s)", RC, ErrMsg);
    error("SQLite failed statement: %s", SQL_CREATE_ANOMALIES_TABLE);

    sqlite3_free(ErrMsg);
    sqlite3_close(Conn);
    Conn = nullptr;
}

Database::~Database() {
    if (!Conn)
        return;

    int RC = sqlite3_close(Conn);
    if (RC != SQLITE_OK)
        error("Could not close connection properly (rc=%d)", RC);
}
