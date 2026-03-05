/*
 * FixHistory
 */

USE [master];
GO

IF DB_ID('FixHistory') IS NOT NULL
BEGIN
	ALTER DATABASE [FixHistory] SET SINGLE_USER WITH ROLLBACK IMMEDIATE;
	DROP DATABASE [FixHistory];
END
GO

USE [FixHistory];
GO

