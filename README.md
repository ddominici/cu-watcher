# cu-watcher

A Go tool that monitors and archives SQL Server Cumulative Updates (CU) and GDR patches by reading the official Microsoft Learn pages, persisting data to SQL Server, and sending email notifications when new releases are detected.

---

## Table of Contents

- [Requirements](#requirements)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Architecture](#architecture)
- [Database Schema](#database-schema)

---

## Requirements

| Component | Minimum Version |
|---|---|
| Go | 1.24 |
| SQL Server | 2017+ |
| Connectivity | HTTPS access to `learn.microsoft.com` and `download.microsoft.com` |

---

## Installation

```bash
git clone <repo-url>
cd cu-watcher
go build -o cu-watcher ./cmd
```

---

## Configuration

Configuration is split into two files:

| File | Contents | Commit? |
|---|---|---|
| `config.yaml` | Non-sensitive parameters | ✅ yes |
| `.env` | Credentials and secrets | ❌ **never** |

### 1. Create the .env file

```bash
cp .env.example .env
```

Fill in the values in `.env`:

```dotenv
# Database
DB_CONNECTION_STRING=sqlserver://user:password@host:1433?database=fixhistory&encrypt=true&trustservercertificate=true

# Email
EMAIL_FROM=cu-watcher@example.com
EMAIL_TO=admin@example.com
EMAIL_SMTP_HOST=smtp.example.com
EMAIL_USERNAME=smtp-user@example.com
EMAIL_PASSWORD=smtp-password
```

The `.env` file is looked up in the same directory as the YAML config file. Variables already present in the system environment always take precedence over `.env`.

### 2. Adapt config.yaml

```yaml
db:
  connectionString: "${DB_CONNECTION_STRING}"   # value from .env

scraper:
  userAgent: "CUWatcher/1.0"
  timeoutSeconds: 60
  maxConcurrency: 4
  delayBetweenRequestsMs: 250
  followKbLinks: true        # download and parse each linked KB article
  maxKbToFetch: 500          # KB article limit per run
  since: "2017-01-01"        # exclude rows with ReleaseDate earlier than this

sources:
  - key: "sql2022"
    majorVersion: 2022
    url: "https://learn.microsoft.com/en-us/troubleshoot/sql/releases/sqlserver-2022/build-versions"
  - key: "sql2019"
    majorVersion: 2019
    url: "https://learn.microsoft.com/en-us/troubleshoot/sql/releases/sqlserver-2019/build-versions"
  # add further versions following the same pattern

logging:
  level: "info"              # debug | info | warn | error
  file: "logs/cu-watcher.log"
  maxSizeMB: 50
  maxBackups: 10
  maxAgeDays: 14

email:
  enabled: false             # set to true to enable notifications
  from: "${EMAIL_FROM}"
  to:
    - "${EMAIL_TO}"
  smtpHost: "${EMAIL_SMTP_HOST}"
  smtpPort: 465              # 587 = STARTTLS, 465 = implicit TLS
  username: "${EMAIL_USERNAME}"
  password: "${EMAIL_PASSWORD}"
  useTLS: true               # true for implicit TLS (port 465)
```

### SMTP Parameters

| `smtpPort` | `useTLS` | Protocol |
|---|---|---|
| 587 | `false` | STARTTLS |
| 465 | `true` | Implicit TLS |

---

## Usage

### First run — create tables

```bash
./cu-watcher --config config.yaml --init-db
```

### Normal run

```bash
./cu-watcher --config config.yaml
```

### Command-line flags

All flags override the corresponding value in the config file.

| Flag | Default | Description |
|---|---|---|
| `--config` | `config.yaml` | Path to the configuration file (yaml/json) |
| `--connection` | *(from config)* | Override the SQL Server connection string |
| `--init-db` | `false` | Create base tables if they do not exist |
| `--only` | *(all)* | Sources to process, e.g. `sql2022,sql2019` |
| `--follow-kb` | *(from config)* | Override follow KB links (`true`/`false`) |
| `--max-kb` | *(from config)* | Override maximum number of KB articles to download |
| `--since` | *(from config)* | Filter rows with `ReleaseDate >= YYYY-MM-DD` |
| `--log-level` | *(from config)* | Override log level |
| `--log-file` | *(from config)* | Override log file path |
| `--notify-latest` | `false` | Send email with the latest CU/GDR per SQL version currently in the DB |

### Examples

```bash
# Process only SQL Server 2022 with verbose logging
./cu-watcher --only sql2022 --log-level debug

# Download only KB articles published after January 2025
./cu-watcher --since 2025-01-01 --follow-kb=true --max-kb 100

# Use a different connection string without changing the config
./cu-watcher --connection "sqlserver://sa:pass@localhost?database=test&trustservercertificate=true"

# Send a digest email with the latest CU/GDR currently stored in the DB (no scraping)
./cu-watcher --notify-latest
```

### Scheduling (Linux/macOS)

```cron
# Run every day at 06:00
0 6 * * * /opt/cu-watcher/cu-watcher --config /opt/cu-watcher/config.yaml >> /var/log/cu-watcher.log 2>&1
```

> Variables defined in `.env` are loaded automatically. Alternatively, export them into the process environment (e.g. via systemd `EnvironmentFile=`).

---

## Architecture

```
cu-watcher/
├── .env.example                 # environment variable template (copy to .env)
├── config.yaml                  # non-sensitive configuration
├── cmd/
│   └── main.go                  # entrypoint, execution flow orchestration
└── internal/
    ├── config/
    │   └── config.go            # Load(): reads .env, expands ${VAR}, parses YAML
    ├── httpx/
    │   └── client.go            # HTTP client with retry (429/5xx), SHA256, logging
    ├── parse/
    │   ├── models.go            # BuildRow, KbArticle, KbFileRecord
    │   ├── build_versions.go    # parser for Microsoft Learn "build versions" pages
    │   ├── kb_article.go        # parser for KB articles (title, sections, links, dates)
    │   ├── kb_files.go          # CSV link extraction, CSV and HTML KB file list parsing
    │   └── util.go              # helpers: clean, parseLearnDate, absURL, ...
    ├── db/
    │   ├── repo.go              # Repository: all database operations
    │   ├── batch_insert.go      # batched multi-row INSERT for BuildRow
    │   ├── schema.go            # DDL for base tables (baseSchemaSQL)
    │   └── sanitize.go          # SQL identifier sanitization
    ├── logging/
    │   └── logging.go           # zap logger (console + rotating file via lumberjack)
    └── notify/
        └── email.go             # SMTP email notification for new releases
```

### Execution flow

```
main()
 │
 ├─ config.Load() → reads .env → expands ${VAR} → parses YAML
 │
 ├─ For each source (sql2022, sql2019, …)
 │   ├─ GET "build-versions" page
 │   ├─ SaveRawPage → dbo.RawPages (skip if SHA256 already present)
 │   ├─ ParseBuildVersions → []BuildRow
 │   ├─ For each topic table (CU_Builds, GDR_Builds, …)
 │   │   ├─ EnsureTopicTable → CREATE TABLE IF NOT EXISTS
 │   │   ├─ FindNewBuildRows → rows not yet in DB (dedup by KbNumber)
 │   │   └─ InsertBuildRowsBatched → dbo.Sql{version}_{topic}
 │   └─ ExtractKbLinks → kbQueue
 │
 ├─ For each KB in kbQueue (up to maxKbToFetch)
 │   ├─ GET KB article
 │   ├─ SaveRawPage → dbo.RawPages
 │   ├─ ParseKbArticle → KbArticle
 │   ├─ UpsertKbArticle → dbo.KbArticles
 │   ├─ HasKbFiles? → skip if already present
 │   ├─ ExtractKbFilesCSVLink → CSV file list URL (SQL 2022/2025)
 │   │   ├─ [CSV found] GET CSV → ParseKbFilesCSV → []KbFileRecord (strip UTF-8 BOM)
 │   │   └─ [no CSV]    ParseKbFilesHTML → []KbFileRecord (SQL 2019/2017 embedded tables)
 │   └─ InsertKbFileRecords → dbo.KbPackageFiles
 │
 └─ SendNewReleases → SMTP email (only if enabled and new rows exist)
```

### Secrets management

```
.env (not committed)           config.yaml (committed)
────────────────────           ───────────────────────
DB_CONNECTION_STRING=…    →    connectionString: "${DB_CONNECTION_STRING}"
EMAIL_USERNAME=…          →    username: "${EMAIL_USERNAME}"
EMAIL_PASSWORD=…          →    password: "${EMAIL_PASSWORD}"
EMAIL_FROM=…              →    from: "${EMAIL_FROM}"
EMAIL_TO=…                →    to: ["${EMAIL_TO}"]
EMAIL_SMTP_HOST=…         →    smtpHost: "${EMAIL_SMTP_HOST}"
```

Resolution priority: **system environment** > `.env` > literal value in `config.yaml`.

### Email notifications

- **`SendNewReleases`**: sent automatically at the end of every run if `email.enabled = true` and new build rows were detected.
- **`SendLatestBuilds`**: sent when `--notify-latest` is passed; emails a snapshot of the latest CU/GDR per SQL version currently stored in the DB, regardless of whether new rows were found.
- Both functions are no-ops when `email.enabled = false` or the recipient list is empty.

### Deduplication

- **RawPages**: unique index on `(SourceKey, SHA256)`. If the page has not changed, the insert is skipped (`IF NOT EXISTS`).
- **BuildRow**: `FindNewBuildRows` queries the DB for already-present `KbNumber` values before inserting; only new rows are accumulated for the email notification.
- **KbPackageFiles**: `HasKbFiles` checks for existing records for that `KbNumber` before downloading the CSV.

### Retry and throttling

The HTTP client retries up to **5 times** with exponential backoff (`attempt² × 300ms`) on network errors and `429` / `5xx` responses. The `delayBetweenRequestsMs` parameter introduces a fixed pause between every request.

---

## Database Schema

### `dbo.RawPages`

Raw snapshot of every downloaded page.

| Column | Type | Notes |
|---|---|---|
| `Id` | BIGINT IDENTITY | PK |
| `SourceKey` | NVARCHAR(200) | e.g. `sql2022-build-versions`, `kb-KB5078297` |
| `Url` | NVARCHAR(1000) | |
| `RetrievedAtUtc` | DATETIME2(3) | |
| `StatusCode` | INT | |
| `ETag` | NVARCHAR(200) | |
| `LastModified` | NVARCHAR(200) | |
| `ContentType` | NVARCHAR(200) | |
| `Sha256` | CHAR(64) | body hash; unique index with SourceKey |
| `Html` | NVARCHAR(MAX) | raw response body |

### `dbo.Sql{version}_{topic}` (e.g. `dbo.Sql2022_CU_Builds`)

One table per `(SQL version, update type)` combination. Possible topics: `CU_Builds`, `GDR_Builds`, `AzureConnectPack_Builds`, `Other_Builds`.

| Column | Type |
|---|---|
| `Id` | BIGINT IDENTITY PK |
| `MajorVersion` | INT |
| `Topic` | NVARCHAR(200) |
| `UpdateName` | NVARCHAR(100) |
| `SqlBuild` | NVARCHAR(50) |
| `SqlFileVersion` | NVARCHAR(50) |
| `AsBuild` | NVARCHAR(50) |
| `AsFileVersion` | NVARCHAR(50) |
| `KbNumber` | NVARCHAR(50) |
| `KbUrl` | NVARCHAR(1000) |
| `ReleaseDate` | DATE |
| `ExtraJson` | NVARCHAR(MAX) |
| `SourceUrl` | NVARCHAR(1000) |
| `RetrievedAtUtc` | DATETIME2(3) |

### `dbo.KbArticles`

Full content of each KB article.

| Column | Type |
|---|---|
| `KbNumber` | NVARCHAR(50) PK |
| `Url` | NVARCHAR(1000) |
| `Title` | NVARCHAR(400) |
| `AppliesTo` | NVARCHAR(200) |
| `ReleaseDate` | DATE |
| `ProductVersion` | NVARCHAR(50) |
| `RetrievedAtUtc` | DATETIME2(3) |
| `ContentText` | NVARCHAR(MAX) |
| `ContentHtml` | NVARCHAR(MAX) |
| `SectionsJson` | NVARCHAR(MAX) |
| `ExtraJson` | NVARCHAR(MAX) |

### `dbo.KbPackageFiles`

Individual files included in each CU/GDR package, extracted from the CSV published by Microsoft.

| Column | Type | Notes |
|---|---|---|
| `Id` | BIGINT IDENTITY PK | |
| `KbNumber` | NVARCHAR(50) | logical FK → KbArticles |
| `Component` | NVARCHAR(200) | e.g. `SQL Server 2022 Database Engine` |
| `FileName` | NVARCHAR(500) | e.g. `sqlservr.exe` |
| `FileVersion` | NVARCHAR(50) | e.g. `2022.160.4236.2` |
| `FileSizeBytes` | BIGINT | |
| `FileDate` | DATE | file build date |
| `Platform` | NVARCHAR(20) | `x64`, `x86`, `n/a` |
| `RetrievedAtUtc` | DATETIME2(3) | |

---

## Dependencies

| Library | Usage |
|---|---|
| `github.com/PuerkitoBio/goquery` | HTML parsing of Microsoft Learn pages |
| `github.com/denisenkom/go-mssqldb` | SQL Server driver for `database/sql` |
| `github.com/spf13/pflag` | CLI argument parsing |
| `go.uber.org/zap` | high-performance structured logging |
| `gopkg.in/natefinch/lumberjack.v2` | automatic log file rotation |
| `gopkg.in/yaml.v3` | YAML/JSON config file parsing |
