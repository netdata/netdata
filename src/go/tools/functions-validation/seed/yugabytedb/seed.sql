CREATE EXTENSION IF NOT EXISTS pg_stat_statements;

CREATE TABLE IF NOT EXISTS items (
  id INT PRIMARY KEY,
  name TEXT
);

INSERT INTO items (id, name) VALUES
  (1, 'alpha'),
  (2, 'beta'),
  (3, 'gamma')
ON CONFLICT (id) DO NOTHING;

SELECT * FROM items WHERE id = 1;
UPDATE items SET name = 'delta' WHERE id = 2;
SELECT count(*) FROM items;
