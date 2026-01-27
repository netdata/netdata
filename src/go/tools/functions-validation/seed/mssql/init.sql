IF DB_ID('netdata') IS NULL
BEGIN
  CREATE DATABASE netdata;
END
GO

ALTER DATABASE netdata SET QUERY_STORE = ON;
GO

IF NOT EXISTS (SELECT 1 FROM sys.server_principals WHERE name = 'netdata_limited')
BEGIN
  CREATE LOGIN netdata_limited WITH PASSWORD = 'Netdata123!';
END
GO

-- Ensure the limited user can start the collector in E2E:
-- A previous DENY will override GRANT, so revoke first.
REVOKE VIEW SERVER STATE FROM netdata_limited;
GO

GRANT VIEW SERVER STATE TO netdata_limited;
GO

USE msdb;
GO

IF NOT EXISTS (SELECT 1 FROM sys.database_principals WHERE name = 'netdata_limited')
BEGIN
  CREATE USER netdata_limited FOR LOGIN netdata_limited;
END
GO

GRANT SELECT ON dbo.sysjobs TO netdata_limited;
GO

USE netdata;
GO

IF NOT EXISTS (SELECT 1 FROM sys.database_principals WHERE name = 'netdata_limited')
BEGIN
  CREATE USER netdata_limited FOR LOGIN netdata_limited;
END
GO

GRANT CONNECT TO netdata_limited;
GO

IF OBJECT_ID('dbo.sample', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.sample (
    id INT IDENTITY(1,1) PRIMARY KEY,
    name NVARCHAR(64) NOT NULL,
    value INT NOT NULL
  );
END
GO

IF OBJECT_ID('dbo.deadlock_a', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.deadlock_a (
    id INT PRIMARY KEY,
    value INT NOT NULL
  );
END
GO

IF OBJECT_ID('dbo.deadlock_b', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.deadlock_b (
    id INT PRIMARY KEY,
    value INT NOT NULL
  );
END
GO

IF NOT EXISTS (SELECT 1 FROM dbo.deadlock_a WHERE id = 1)
BEGIN
  INSERT INTO dbo.deadlock_a (id, value) VALUES (1, 10);
END
GO

IF NOT EXISTS (SELECT 1 FROM dbo.deadlock_b WHERE id = 1)
BEGIN
  INSERT INTO dbo.deadlock_b (id, value) VALUES (1, 20);
END
GO

-- Table for error category testing (constraint violations, data type errors).
IF OBJECT_ID('dbo.error_test', 'U') IS NULL
BEGIN
  CREATE TABLE dbo.error_test (
    id INT PRIMARY KEY,
    unique_col NVARCHAR(64) NOT NULL UNIQUE,
    int_col INT NOT NULL
  );
END
GO

IF NOT EXISTS (SELECT 1 FROM dbo.error_test WHERE id = 1)
BEGIN
  INSERT INTO dbo.error_test (id, unique_col, int_col) VALUES (1, 'existing_value', 100);
END
GO

INSERT INTO dbo.sample (name, value)
VALUES ('alpha', 10), ('beta', 20), ('gamma', 30);
GO

SELECT COUNT(*) FROM dbo.sample;
SELECT * FROM dbo.sample WHERE value > 15;
UPDATE dbo.sample SET value = value + 1 WHERE name = 'alpha';
GO
