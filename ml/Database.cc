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

const char *ml::Database::SQL_CREATE_ANOMALY_RATE_INFO_TABLE =
    "CREATE TABLE IF NOT EXISTS anomaly_rate_info( "
    "     host_id text NOT NULL, "
    "     after int NOT NULL, "
    "     before int NOT NULL, "
    "     anomaly_rates text "
    ");";

const char *ml::Database::SQL_INSERT_BULK_ANOMALY_RATE_INFO =
    "INSERT INTO anomaly_rate_info( "
    "     host_id, after, before, anomaly_rates) "
    " VALUES (?1, ?2, ?3, ?4);";
   
const char *ml::Database::SQL_SELECT_ANOMALY_RATE_INFO =
    "SELECT  main.dimension_id, COALESCE("
    " ((COALESCE(pre.avg,0) * (SELECT before-?2 FROM anomaly_rate_info WHERE after < ?2 ORDER BY after DESC LIMIT 1)) + "
    " (COALESCE(main.avg,0) * ((SELECT before FROM anomaly_rate_info WHERE before <= ?3 ORDER BY before DESC LIMIT 1) - "
    "              (SELECT after FROM anomaly_rate_info WHERE after >= ?2 ORDER BY after ASC LIMIT 1))) + "
    " (COALESCE(post.avg,0) * (SELECT ?3-after FROM anomaly_rate_info WHERE before > ?3 OR "
    "                ((SELECT before FROM anomaly_rate_info WHERE before <= ?3 ORDER BY before DESC LIMIT 1) "
    "                 AND NOT EXISTS (SELECT before FROM anomaly_rate_info WHERE before > ?3 ORDER BY before ASC LIMIT 1)) "
    "              ORDER BY before ASC LIMIT 1))) / ( "
    "  (SELECT before-?2 FROM anomaly_rate_info WHERE after < ?2 ORDER BY after DESC LIMIT 1) + "
    "  (SELECT before FROM anomaly_rate_info WHERE before <= ?3 ORDER BY before DESC LIMIT 1) - "
    "  (SELECT after FROM anomaly_rate_info WHERE after >= ?2 ORDER BY after ASC LIMIT 1) + "
    "  (SELECT ?3-after FROM anomaly_rate_info WHERE before > ?3 OR "
    "                 ((SELECT before FROM anomaly_rate_info WHERE before <= ?3 ORDER BY before DESC LIMIT 1) "
    "                  AND NOT EXISTS (SELECT before FROM anomaly_rate_info WHERE before > ?3 ORDER BY before ASC LIMIT 1)) "
    "               ORDER BY before ASC LIMIT 1) "
    " ),0.0) percentage FROM "
    "(SELECT dimension_id, AVG(anomaly_percentage) avg FROM "
    "  (SELECT json_extract(j.value, '$[1]') AS dimension_id, "
    "  json_extract(j.value, '$[0]') AS anomaly_percentage "
    "  FROM anomaly_rate_info AS ari, json_each(ari.anomaly_rates) AS j " 
    "  WHERE ari.host_id == ?1 AND ari.after >= ?2 AND ari.before <= ?3 "
    "  AND json_valid(ari.anomaly_rates)) "
    "  GROUP BY dimension_id) AS main "
    " LEFT JOIN "
    "(SELECT dimension_id, AVG(anomaly_percentage) avg FROM "
    "  (SELECT json_extract(j.value, '$[1]') AS dimension_id, "
    "  json_extract(j.value, '$[0]') AS anomaly_percentage "
    "  FROM anomaly_rate_info AS ari, json_each(ari.anomaly_rates) AS j " 
    "  WHERE ari.host_id == ?1 "
    "  AND ari.after >= (SELECT after FROM anomaly_rate_info WHERE after <= ?2 ORDER BY after DESC LIMIT 1) "
    "  AND ari.before <= (SELECT before FROM anomaly_rate_info WHERE before >= ?2 ORDER BY before ASC LIMIT 1) "
    "  AND json_valid(ari.anomaly_rates)) "
    "  GROUP BY dimension_id) AS pre "
    " ON main.dimension_id = pre.dimension_id "
    " LEFT JOIN "
    "(SELECT dimension_id, AVG(anomaly_percentage) avg FROM "
    "  (SELECT json_extract(j.value, '$[1]') AS dimension_id, "
    "  json_extract(j.value, '$[0]') AS anomaly_percentage "
    "  FROM anomaly_rate_info AS ari, json_each(ari.anomaly_rates) AS j " 
    "  WHERE ari.host_id == ?1 "
    "  AND ari.after >= (SELECT after FROM anomaly_rate_info WHERE after <= ?3 ORDER BY after DESC LIMIT 1) "
    "  AND ari.before <= (SELECT before FROM anomaly_rate_info WHERE before >= ?3 OR "
    "                ((SELECT before FROM anomaly_rate_info WHERE before <= ?3 ORDER BY before DESC LIMIT 1) "
    "                 AND NOT EXISTS (SELECT before FROM anomaly_rate_info WHERE before >= ?3 ORDER BY before ASC LIMIT 1)) "
    "              ORDER BY before DESC LIMIT 1) "
    "  AND json_valid(ari.anomaly_rates)) "
    "  GROUP BY dimension_id) AS post "
    " ON main.dimension_id = post.dimension_id "
    "GROUP BY main.dimension_id HAVING percentage > 0.0;";
 
const char *ml::Database::SQL_SELECT_ANOMALY_RATE_INFO_RANGE =
    "SELECT after, before FROM anomaly_rate_info WHERE host_id == ?1 ORDER BY before DESC LIMIT 1;";

const char *ml::Database::SQL_REMOVE_OLD_ANOMALY_RATE_INFO =
    "DELETE FROM anomaly_rate_info "
    " WHERE before <= ?1;";

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
    RC = sqlite3_exec(Conn, SQL_CREATE_ANOMALIES_TABLE, nullptr, nullptr, &ErrMsg);
    if (RC == SQLITE_OK) {
        
        RC = sqlite3_exec(Conn, SQL_CREATE_ANOMALY_RATE_INFO_TABLE, nullptr, nullptr, &ErrMsg);
        if (RC == SQLITE_OK) {
            return;
        }
        else {
            error("SQLite error during database initialization; creating table anomaly_rate_info, rc = %d (%s)", RC, ErrMsg);
            error("SQLite failed statement: %s", SQL_CREATE_ANOMALY_RATE_INFO_TABLE);
        }
    }
    else {
        error("SQLite error during database initialization; creating table anomaly_events, rc = %d (%s)", RC, ErrMsg);
        error("SQLite failed statement: %s", SQL_CREATE_ANOMALIES_TABLE);
    }

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
