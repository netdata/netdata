CREATE TABLE IF NOT EXISTS sample (
  id INT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  value INT NOT NULL
);

-- Tables for deadlock induction tests.
CREATE TABLE IF NOT EXISTS deadlock_a (
  id INT PRIMARY KEY,
  value INT NOT NULL
) ENGINE=InnoDB;

CREATE TABLE IF NOT EXISTS deadlock_b (
  id INT PRIMARY KEY,
  value INT NOT NULL
) ENGINE=InnoDB;

INSERT INTO deadlock_a (id, value) VALUES (1, 10);
INSERT INTO deadlock_b (id, value) VALUES (1, 20);

-- Ensure statement digest collection is enabled.
UPDATE performance_schema.setup_consumers
  SET ENABLED = 'YES'
  WHERE NAME IN (
    'events_statements_summary_by_digest',
    'events_statements_summary_by_program',
    'events_statements_summary_by_user_by_event_name',
    'events_statements_summary_by_host_by_event_name',
    'events_statements_summary_by_thread_by_event_name'
  );
UPDATE performance_schema.setup_instruments
  SET ENABLED = 'YES', TIMED = 'YES'
  WHERE NAME LIKE 'statement/%';

GRANT USAGE, REPLICATION CLIENT, PROCESS ON *.* TO 'netdata'@'%';
GRANT SELECT ON performance_schema.* TO 'netdata'@'%';
FLUSH PRIVILEGES;

INSERT INTO sample (name, value)
VALUES
  ('alpha', 10),
  ('beta', 20),
  ('gamma', 30);

SELECT COUNT(*) FROM sample;
SELECT * FROM sample WHERE value > 15;
UPDATE sample SET value = value + 1 WHERE name = 'alpha';
SELECT * FROM sample WHERE name = 'beta';
