#!/usr/bin/env python3

"""
Synthetic load generator for a PostgreSQL instance.

Creates a test table and runs a mix of INSERTs, SELECTs, UPDATEs, DELETEs,
and transactions (with occasional rollbacks) to exercise the OTel PostgreSQL
receiver metrics: postgresql.commits, postgresql.rollbacks, postgresql.db.size,
postgresql.rows, postgresql.operations, postgresql.blocks_read, etc.

Requirements:
    pip install psycopg2-binary

Usage:
    python postgresql_load_generator.py [--host localhost] [--port 5432] \
        [--user otel] [--password otel] [--dbname testdb] [--rate 100]

    --rate  Target operations per second (approximate). Default: 100.
"""

import argparse
import random
import string
import time

import psycopg2


SETUP_SQL = """
CREATE TABLE IF NOT EXISTS load_test (
    id SERIAL PRIMARY KEY,
    key TEXT NOT NULL,
    value TEXT,
    counter INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_load_test_key ON load_test (key);
"""


def random_string(length=32):
    return "".join(random.choices(string.ascii_letters + string.digits, k=length))


def setup(conn):
    with conn.cursor() as cur:
        cur.execute(SETUP_SQL)
    conn.commit()
    print("Table 'load_test' ready.")


def run(host, port, user, password, dbname, rate):
    conn = psycopg2.connect(host=host, port=port, user=user, password=password, dbname=dbname)
    conn.autocommit = False
    print(f"Connected to PostgreSQL at {host}:{port}/{dbname}")

    setup(conn)

    print(f"Target rate: ~{rate} ops/s  (Ctrl+C to stop)\n")

    ops = 0
    start = time.monotonic()
    interval = 1.0 / rate if rate > 0 else 0

    try:
        while True:
            op = random.choices(
                ["insert", "select", "update", "delete", "rollback", "index_scan"],
                weights=[30, 25, 20, 10, 5, 10],
            )[0]

            try:
                with conn.cursor() as cur:
                    if op == "insert":
                        cur.execute(
                            "INSERT INTO load_test (key, value, counter) VALUES (%s, %s, %s)",
                            (f"key:{random.randint(0, 500)}", random_string(random.randint(32, 256)), 0),
                        )
                        conn.commit()

                    elif op == "select":
                        cur.execute(
                            "SELECT * FROM load_test WHERE key = %s",
                            (f"key:{random.randint(0, 500)}",),
                        )
                        cur.fetchall()
                        conn.commit()

                    elif op == "update":
                        cur.execute(
                            "UPDATE load_test SET counter = counter + 1, value = %s WHERE key = %s",
                            (random_string(64), f"key:{random.randint(0, 500)}"),
                        )
                        conn.commit()

                    elif op == "delete":
                        cur.execute(
                            "DELETE FROM load_test WHERE key = %s AND id IN (SELECT id FROM load_test WHERE key = %s LIMIT 1)",
                            (f"key:{random.randint(0, 500)}", f"key:{random.randint(0, 500)}"),
                        )
                        conn.commit()

                    elif op == "rollback":
                        cur.execute(
                            "INSERT INTO load_test (key, value) VALUES (%s, %s)",
                            (f"rollback:{random.randint(0, 100)}", random_string(64)),
                        )
                        conn.rollback()

                    elif op == "index_scan":
                        cur.execute(
                            "SELECT count(*) FROM load_test WHERE key BETWEEN %s AND %s",
                            (f"key:{random.randint(0, 250)}", f"key:{random.randint(250, 500)}"),
                        )
                        cur.fetchone()
                        conn.commit()

            except psycopg2.Error:
                conn.rollback()

            ops += 1

            if ops % 500 == 0:
                elapsed = time.monotonic() - start
                print(f"  {ops} ops in {elapsed:.1f}s  ({ops / elapsed:.0f} ops/s)")

            if interval:
                time.sleep(interval)

    except KeyboardInterrupt:
        elapsed = time.monotonic() - start
        print(f"\nDone. {ops} operations in {elapsed:.1f}s ({ops / elapsed:.0f} ops/s)")
    finally:
        conn.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="PostgreSQL synthetic load generator")
    parser.add_argument("--host", default="localhost")
    parser.add_argument("--port", type=int, default=5432)
    parser.add_argument("--user", default="otel")
    parser.add_argument("--password", default="otel")
    parser.add_argument("--dbname", default="testdb")
    parser.add_argument("--rate", type=int, default=100, help="Target ops/sec")
    args = parser.parse_args()

    run(args.host, args.port, args.user, args.password, args.dbname, args.rate)
