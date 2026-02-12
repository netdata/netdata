CREATE DATABASE IF NOT EXISTS netdata;
USE netdata;

CREATE TABLE IF NOT EXISTS items (
  id INT PRIMARY KEY,
  name STRING
);

UPSERT INTO items (id, name) VALUES
  (1, 'alpha'),
  (2, 'beta'),
  (3, 'gamma');

SELECT * FROM items WHERE id = 1;
UPDATE items SET name = 'delta' WHERE id = 2;
SELECT count(*) FROM items;
