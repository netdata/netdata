SET PAGESIZE 0
SET FEEDBACK OFF

SELECT COUNT(*) FROM netdata.demo;
SELECT COUNT(*) FROM netdata.demo;
SELECT COUNT(*) FROM netdata.demo;
SELECT COUNT(*) FROM netdata.demo;
SELECT COUNT(*) FROM netdata.demo;

SELECT name FROM netdata.demo WHERE id = 1;
SELECT name FROM netdata.demo WHERE id = 1;
SELECT name FROM netdata.demo WHERE id = 1;

UPDATE netdata.demo SET name = 'beta' WHERE id = 1;
UPDATE netdata.demo SET name = 'alpha' WHERE id = 1;

COMMIT;
