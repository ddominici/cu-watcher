package db

const baseSchemaSQL = `
IF OBJECT_ID('dbo.RawPages') IS NULL
BEGIN
  CREATE TABLE dbo.RawPages (
    Id BIGINT IDENTITY(1,1) PRIMARY KEY,
    SourceKey NVARCHAR(200) NOT NULL,
    Url NVARCHAR(1000) NOT NULL,
    RetrievedAtUtc DATETIME2(3) NOT NULL,
    StatusCode INT NULL,
    ETag NVARCHAR(200) NULL,
    LastModified NVARCHAR(200) NULL,
    ContentType NVARCHAR(200) NULL,
    Sha256 CHAR(64) NOT NULL,
    Html NVARCHAR(MAX) NOT NULL
  );
  CREATE UNIQUE INDEX IX_RawPages_SourceKey_Sha256 ON dbo.RawPages(SourceKey, Sha256);
END;

IF OBJECT_ID('dbo.KbPackageFiles') IS NULL
BEGIN
  CREATE TABLE dbo.KbPackageFiles (
    Id             BIGINT IDENTITY(1,1) PRIMARY KEY,
    KbNumber       NVARCHAR(50)   NOT NULL,
    Component      NVARCHAR(200)  NULL,
    FileName       NVARCHAR(500)  NOT NULL,
    FileVersion    NVARCHAR(50)   NULL,
    FileSizeBytes  BIGINT         NULL,
    FileDate       DATE           NULL,
    Platform       NVARCHAR(20)   NULL,
    RetrievedAtUtc DATETIME2(3)   NOT NULL
  );
  CREATE INDEX IX_KbPackageFiles_KbNumber ON dbo.KbPackageFiles(KbNumber);
END;

IF OBJECT_ID('dbo.KbArticles') IS NULL
BEGIN
  CREATE TABLE dbo.KbArticles (
    KbNumber NVARCHAR(50) NOT NULL PRIMARY KEY,
    Url NVARCHAR(1000) NOT NULL,
    Title NVARCHAR(400) NULL,
    AppliesTo NVARCHAR(200) NULL,
    ReleaseDate DATE NULL,
    ProductVersion NVARCHAR(50) NULL,
    RetrievedAtUtc DATETIME2(3) NOT NULL,
    ContentText NVARCHAR(MAX) NULL,
    ContentHtml NVARCHAR(MAX) NULL,
    SectionsJson NVARCHAR(MAX) NULL,
    ExtraJson NVARCHAR(MAX) NULL
  );
END;
`
