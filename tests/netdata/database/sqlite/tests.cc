#include <gtest/gtest.h>
#include "daemon/common.h"

TEST(sqlite, statements) {
    sqlite3 *DB;
    int Ret;

    Ret = sqlite3_open(":memory:", &DB);
    EXPECT_EQ(Ret, SQLITE_OK);

    {
        Ret = sqlite3_exec_monitored(DB, "CREATE TABLE IF NOT EXISTS mine (id1, id2);", 0, 0, NULL);
        EXPECT_EQ(Ret, SQLITE_OK);

        Ret = sqlite3_exec_monitored(DB, "DELETE FROM MINE LIMIT 1;", 0, 0, NULL);
        EXPECT_EQ(Ret, SQLITE_OK);

        Ret = sqlite3_exec_monitored(DB, "UPDATE MINE SET id1=1 LIMIT 1;", 0, 0, NULL);
        EXPECT_EQ(Ret, SQLITE_OK);
    }

    {
        BUFFER *Stmt = buffer_create(ACLK_SYNC_QUERY_SIZE);
        const char *UUID = "0000_000";

        buffer_sprintf(Stmt, TABLE_ACLK_ALERT, UUID);
        Ret = sqlite3_exec_monitored(DB, buffer_tostring(Stmt), 0, 0, NULL);
        EXPECT_EQ(Ret, SQLITE_OK);
        buffer_flush(Stmt);

        buffer_sprintf(Stmt, INDEX_ACLK_ALERT, UUID, UUID);
        Ret = sqlite3_exec_monitored(DB, buffer_tostring(Stmt), 0, 0, NULL);
        EXPECT_EQ(Ret, SQLITE_OK);
        buffer_flush(Stmt);

        buffer_free(Stmt);
    }

    Ret = sqlite3_close(DB);
    EXPECT_EQ(Ret, SQLITE_OK);
}
