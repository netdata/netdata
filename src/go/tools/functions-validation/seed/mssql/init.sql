IF DB_ID('netdata') IS NULL
BEGIN
  CREATE DATABASE netdata;
END
GO

ALTER DATABASE netdata SET QUERY_STORE = ON;
GO

USE netdata;
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

INSERT INTO dbo.sample (name, value)
VALUES ('alpha', 10), ('beta', 20), ('gamma', 30);
GO

SELECT COUNT(*) FROM dbo.sample;
SELECT * FROM dbo.sample WHERE value > 15;
UPDATE dbo.sample SET value = value + 1 WHERE name = 'alpha';
GO
