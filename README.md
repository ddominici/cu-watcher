# cu-watcher

A Go tool that monitors and archives SQL Server Cumulative Updates (CU) and GDR patches by reading the official Microsoft Learn pages, persisting data to SQL Server, and sending email notifications when new releases are detected.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
- [Architecture](#architecture)
- [Database Schema](#database-schema)
- [Documentazione in italiano](#documentazione-in-italiano)

---

## Prerequisites

### Build

| Requirement | Notes |
|---|---|
| Go 1.24+ | Required to compile from source. Download from [go.dev/dl](https://go.dev/dl/). |
| Git | Required to clone the repository. |
| Make | Optional. Only needed to run `make all` for cross-platform compilation. |

### Runtime

| Requirement | Notes |
|---|---|
| SQL Server 2017+ | The target database must already exist. cu-watcher creates its own tables (`--init-db`) but does not create the database itself. |
| Outbound HTTPS | Port 443 access to `learn.microsoft.com` and `download.microsoft.com` is required for scraping. |
| Write access on the DB | The configured account needs `CREATE TABLE`, `INSERT`, `UPDATE`, `SELECT` on the target database. |

> **First run:** always pass `--init-db` once to create the required tables before the normal scraping run.

### Email notifications (optional)

Required only when `email.enabled: true` is set in `config.yaml`.

| Requirement | Notes |
|---|---|
| SMTP server | Accessible on the configured port (587 for STARTTLS, 465 for implicit TLS). |
| Valid SMTP credentials | `EMAIL_USERNAME` and `EMAIL_PASSWORD` in `.env`. |

### Windows Authentication (optional)

Required only when `windowsAuth: true` is set or `--windows-auth` is passed.

| Platform | Requirement |
|---|---|
| Windows | Nothing extra — SSPI is built into the OS. The process must run as the Windows user that has SQL Server access. |
| Linux | Kerberos client: `sudo apt install krb5-user` (Debian/Ubuntu) or `sudo dnf install krb5-workstation` (RHEL/Fedora). A valid ticket must be obtained with `kinit user@DOMAIN` before running. |
| macOS | Kerberos is pre-installed. Run `kinit user@DOMAIN` before running the tool. |

---

## Installation

```bash
git clone <repo-url>
cd cu-watcher
go build -o cu-watcher ./cmd
```

To cross-compile for all supported platforms:

```bash
make all   # produces darwin/linux/windows binaries in _releases/
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
# SQL Authentication
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
  windowsAuth: false                            # set to true for Windows Authentication

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

### Windows Authentication

Set `windowsAuth: true` in `config.yaml` (or pass `--windows-auth` on the command line). The connection string must omit the username and password; `IntegratedSecurity=true` is appended automatically.

```dotenv
# .env — no credentials needed
DB_CONNECTION_STRING=sqlserver://localhost:1433?database=fixhistory&encrypt=true&trustservercertificate=true
```

```yaml
# config.yaml
db:
  connectionString: "${DB_CONNECTION_STRING}"
  windowsAuth: true
```

| Platform | Mechanism |
|---|---|
| Windows | SSPI — uses the current process's Windows identity |
| Linux / macOS | Kerberos — requires a valid ticket (`kinit` before running) |

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
| `--windows-auth` | `false` | Use Windows Authentication (IntegratedSecurity=true) |
| `--notify-latest` | `false` | Send email with the latest CU/GDR per SQL version currently in the DB |

### Examples

```bash
# Process only SQL Server 2022 with verbose logging
./cu-watcher --only sql2022 --log-level debug

# Download only KB articles published after January 2025
./cu-watcher --since 2025-01-01 --follow-kb=true --max-kb 100

# Use a different connection string without changing the config
./cu-watcher --connection "sqlserver://sa:pass@localhost?database=test&trustservercertificate=true"

# Connect using the current Windows user (SSPI)
./cu-watcher --windows-auth --connection "sqlserver://localhost?database=fixhistory&trustservercertificate=true"

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
 │  └─ if windowsAuth → withIntegratedSecurity() appends IntegratedSecurity=true
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

---

---

# Documentazione in italiano

Uno strumento Go che monitora e archivia gli aggiornamenti cumulativi (CU) e le patch GDR di SQL Server leggendo le pagine ufficiali di Microsoft Learn, salvando i dati su SQL Server e inviando notifiche email quando vengono rilevate nuove release.

---

## Indice

- [Prerequisiti](#prerequisiti)
- [Installazione](#installazione)
- [Configurazione](#configurazione)
- [Utilizzo](#utilizzo)
- [Architettura](#architettura)
- [Schema del database](#schema-del-database)

---

## Prerequisiti

### Compilazione

| Requisito | Note |
|---|---|
| Go 1.24+ | Necessario per compilare il sorgente. Scaricabile da [go.dev/dl](https://go.dev/dl/). |
| Git | Necessario per clonare il repository. |
| Make | Opzionale. Serve solo per eseguire `make all` e produrre i binari per tutte le piattaforme. |

### Esecuzione

| Requisito | Note |
|---|---|
| SQL Server 2017+ | Il database di destinazione deve già esistere. cu-watcher crea le proprie tabelle (`--init-db`) ma non crea il database. |
| Accesso HTTPS in uscita | Porta 443 verso `learn.microsoft.com` e `download.microsoft.com` per lo scraping. |
| Permessi sul database | L'account configurato deve avere `CREATE TABLE`, `INSERT`, `UPDATE`, `SELECT` sul database di destinazione. |

> **Prima esecuzione:** passare sempre `--init-db` una volta per creare le tabelle necessarie prima dell'esecuzione normale.

### Notifiche email (opzionale)

Necessario solo quando `email.enabled: true` è impostato in `config.yaml`.

| Requisito | Note |
|---|---|
| Server SMTP | Accessibile sulla porta configurata (587 per STARTTLS, 465 per TLS implicito). |
| Credenziali SMTP valide | `EMAIL_USERNAME` e `EMAIL_PASSWORD` nel file `.env`. |

### Autenticazione Windows (opzionale)

Necessario solo quando `windowsAuth: true` è impostato oppure si passa `--windows-auth`.

| Piattaforma | Requisito |
|---|---|
| Windows | Niente di extra — SSPI è integrato nel sistema operativo. Il processo deve essere eseguito come l'utente Windows che ha accesso a SQL Server. |
| Linux | Client Kerberos: `sudo apt install krb5-user` (Debian/Ubuntu) oppure `sudo dnf install krb5-workstation` (RHEL/Fedora). Ottenere un ticket valido con `kinit utente@DOMINIO` prima dell'esecuzione. |
| macOS | Kerberos è preinstallato. Eseguire `kinit utente@DOMINIO` prima di avviare lo strumento. |

---

## Installazione

```bash
git clone <repo-url>
cd cu-watcher
go build -o cu-watcher ./cmd
```

Per compilare per tutte le piattaforme supportate:

```bash
make all   # produce i binari per darwin/linux/windows in _releases/
```

---

## Configurazione

La configurazione è suddivisa in due file:

| File | Contenuto | Committare? |
|---|---|---|
| `config.yaml` | Parametri non sensibili | ✅ sì |
| `.env` | Credenziali e segreti | ❌ **mai** |

### 1. Creare il file .env

```bash
cp .env.example .env
```

Compilare i valori nel file `.env`:

```dotenv
# Autenticazione SQL
DB_CONNECTION_STRING=sqlserver://user:password@host:1433?database=fixhistory&encrypt=true&trustservercertificate=true

# Email
EMAIL_FROM=cu-watcher@example.com
EMAIL_TO=admin@example.com
EMAIL_SMTP_HOST=smtp.example.com
EMAIL_USERNAME=smtp-user@example.com
EMAIL_PASSWORD=smtp-password
```

Il file `.env` viene cercato nella stessa directory del file di configurazione YAML. Le variabili già presenti nell'ambiente di sistema hanno sempre la precedenza su quelle definite in `.env`.

### 2. Adattare config.yaml

```yaml
db:
  connectionString: "${DB_CONNECTION_STRING}"   # valore dal .env
  windowsAuth: false                            # true per l'autenticazione Windows

scraper:
  userAgent: "CUWatcher/1.0"
  timeoutSeconds: 60
  maxConcurrency: 4
  delayBetweenRequestsMs: 250
  followKbLinks: true        # scarica e analizza ogni articolo KB collegato
  maxKbToFetch: 500          # limite di articoli KB per esecuzione
  since: "2017-01-01"        # esclude le righe con ReleaseDate precedente a questa data

sources:
  - key: "sql2022"
    majorVersion: 2022
    url: "https://learn.microsoft.com/en-us/troubleshoot/sql/releases/sqlserver-2022/build-versions"
  - key: "sql2019"
    majorVersion: 2019
    url: "https://learn.microsoft.com/en-us/troubleshoot/sql/releases/sqlserver-2019/build-versions"
  # aggiungere ulteriori versioni seguendo lo stesso schema

logging:
  level: "info"              # debug | info | warn | error
  file: "logs/cu-watcher.log"
  maxSizeMB: 50
  maxBackups: 10
  maxAgeDays: 14

email:
  enabled: false             # impostare a true per abilitare le notifiche
  from: "${EMAIL_FROM}"
  to:
    - "${EMAIL_TO}"
  smtpHost: "${EMAIL_SMTP_HOST}"
  smtpPort: 465              # 587 = STARTTLS, 465 = TLS implicito
  username: "${EMAIL_USERNAME}"
  password: "${EMAIL_PASSWORD}"
  useTLS: true               # true per TLS implicito (porta 465)
```

### Autenticazione Windows

Impostare `windowsAuth: true` in `config.yaml` (oppure passare `--windows-auth` da riga di comando). La connection string non deve contenere username e password; `IntegratedSecurity=true` viene aggiunto automaticamente.

```dotenv
# .env — nessuna credenziale necessaria
DB_CONNECTION_STRING=sqlserver://localhost:1433?database=fixhistory&encrypt=true&trustservercertificate=true
```

```yaml
# config.yaml
db:
  connectionString: "${DB_CONNECTION_STRING}"
  windowsAuth: true
```

| Piattaforma | Meccanismo |
|---|---|
| Windows | SSPI — utilizza l'identità Windows del processo corrente |
| Linux / macOS | Kerberos — richiede un ticket valido (`kinit` prima dell'esecuzione) |

### Parametri SMTP

| `smtpPort` | `useTLS` | Protocollo |
|---|---|---|
| 587 | `false` | STARTTLS |
| 465 | `true` | TLS implicito |

---

## Utilizzo

### Prima esecuzione — creazione delle tabelle

```bash
./cu-watcher --config config.yaml --init-db
```

### Esecuzione normale

```bash
./cu-watcher --config config.yaml
```

### Flag da riga di comando

Tutti i flag sovrascrivono il valore corrispondente nel file di configurazione.

| Flag | Default | Descrizione |
|---|---|---|
| `--config` | `config.yaml` | Percorso del file di configurazione (yaml/json) |
| `--connection` | *(dal config)* | Sovrascrive la connection string di SQL Server |
| `--init-db` | `false` | Crea le tabelle base se non esistono |
| `--only` | *(tutte)* | Sorgenti da elaborare, es. `sql2022,sql2019` |
| `--follow-kb` | *(dal config)* | Sovrascrive il flag per seguire i link KB (`true`/`false`) |
| `--max-kb` | *(dal config)* | Sovrascrive il numero massimo di articoli KB da scaricare |
| `--since` | *(dal config)* | Filtra le righe con `ReleaseDate >= YYYY-MM-DD` |
| `--log-level` | *(dal config)* | Sovrascrive il livello di log |
| `--log-file` | *(dal config)* | Sovrascrive il percorso del file di log |
| `--windows-auth` | `false` | Usa l'autenticazione Windows (IntegratedSecurity=true) |
| `--notify-latest` | `false` | Invia una email con l'ultimo CU/GDR per versione SQL presente nel DB |

### Esempi

```bash
# Elabora solo SQL Server 2022 con log dettagliato
./cu-watcher --only sql2022 --log-level debug

# Scarica solo gli articoli KB pubblicati dopo gennaio 2025
./cu-watcher --since 2025-01-01 --follow-kb=true --max-kb 100

# Utilizza una connection string diversa senza modificare il config
./cu-watcher --connection "sqlserver://sa:pass@localhost?database=test&trustservercertificate=true"

# Connessione con l'utente Windows corrente (SSPI)
./cu-watcher --windows-auth --connection "sqlserver://localhost?database=fixhistory&trustservercertificate=true"

# Invia una email di riepilogo con l'ultimo CU/GDR attualmente nel DB (senza scraping)
./cu-watcher --notify-latest
```

### Pianificazione (Linux/macOS)

```cron
# Esecuzione ogni giorno alle 06:00
0 6 * * * /opt/cu-watcher/cu-watcher --config /opt/cu-watcher/config.yaml >> /var/log/cu-watcher.log 2>&1
```

> Le variabili definite in `.env` vengono caricate automaticamente. In alternativa, esportarle nell'ambiente del processo (ad esempio tramite `EnvironmentFile=` di systemd).

---

## Architettura

```
cu-watcher/
├── .env.example                 # template delle variabili d'ambiente (copiare in .env)
├── config.yaml                  # configurazione non sensibile
├── cmd/
│   └── main.go                  # entrypoint, orchestrazione del flusso di esecuzione
└── internal/
    ├── config/
    │   └── config.go            # Load(): legge .env, espande ${VAR}, analizza YAML
    ├── httpx/
    │   └── client.go            # client HTTP con retry (429/5xx), SHA256, logging
    ├── parse/
    │   ├── models.go            # BuildRow, KbArticle, KbFileRecord
    │   ├── build_versions.go    # parser per le pagine "build versions" di Microsoft Learn
    │   ├── kb_article.go        # parser per gli articoli KB (titolo, sezioni, link, date)
    │   ├── kb_files.go          # estrazione link CSV, parsing CSV e tabelle HTML KB
    │   └── util.go              # helper: clean, parseLearnDate, absURL, …
    ├── db/
    │   ├── repo.go              # Repository: tutte le operazioni sul database
    │   ├── batch_insert.go      # INSERT multi-riga in batch per BuildRow
    │   ├── schema.go            # DDL per le tabelle base (baseSchemaSQL)
    │   └── sanitize.go          # sanitizzazione degli identificatori SQL
    ├── logging/
    │   └── logging.go           # logger zap (console + file rotante via lumberjack)
    └── notify/
        └── email.go             # notifica email SMTP per le nuove release
```

### Flusso di esecuzione

```
main()
 │
 ├─ config.Load() → legge .env → espande ${VAR} → analizza YAML
 │  └─ se windowsAuth → withIntegratedSecurity() aggiunge IntegratedSecurity=true
 │
 ├─ Per ogni sorgente (sql2022, sql2019, …)
 │   ├─ GET pagina "build-versions"
 │   ├─ SaveRawPage → dbo.RawPages (salta se SHA256 già presente)
 │   ├─ ParseBuildVersions → []BuildRow
 │   ├─ Per ogni tabella topic (CU_Builds, GDR_Builds, …)
 │   │   ├─ EnsureTopicTable → CREATE TABLE IF NOT EXISTS
 │   │   ├─ FindNewBuildRows → righe non ancora nel DB (dedup per KbNumber)
 │   │   └─ InsertBuildRowsBatched → dbo.Sql{versione}_{topic}
 │   └─ ExtractKbLinks → kbQueue
 │
 ├─ Per ogni KB nella kbQueue (fino a maxKbToFetch)
 │   ├─ GET articolo KB
 │   ├─ SaveRawPage → dbo.RawPages
 │   ├─ ParseKbArticle → KbArticle
 │   ├─ UpsertKbArticle → dbo.KbArticles
 │   ├─ HasKbFiles? → salta se già presente
 │   ├─ ExtractKbFilesCSVLink → URL del CSV (SQL 2022/2025)
 │   │   ├─ [CSV trovato] GET CSV → ParseKbFilesCSV → []KbFileRecord (rimuove BOM UTF-8)
 │   │   └─ [no CSV]      ParseKbFilesHTML → []KbFileRecord (tabelle embedded per SQL 2019/2017)
 │   └─ InsertKbFileRecords → dbo.KbPackageFiles
 │
 └─ SendNewReleases → email SMTP (solo se abilitato e ci sono nuove righe)
```

### Gestione dei segreti

```
.env (non committato)          config.yaml (committato)
─────────────────────          ──────────────────────────
DB_CONNECTION_STRING=…    →    connectionString: "${DB_CONNECTION_STRING}"
EMAIL_USERNAME=…          →    username: "${EMAIL_USERNAME}"
EMAIL_PASSWORD=…          →    password: "${EMAIL_PASSWORD}"
EMAIL_FROM=…              →    from: "${EMAIL_FROM}"
EMAIL_TO=…                →    to: ["${EMAIL_TO}"]
EMAIL_SMTP_HOST=…         →    smtpHost: "${EMAIL_SMTP_HOST}"
```

Priorità di risoluzione: **variabile di sistema** > `.env` > valore letterale in `config.yaml`.

### Notifiche email

- **`SendNewReleases`**: inviata automaticamente al termine di ogni esecuzione se `email.enabled = true` e vengono rilevate nuove righe di build.
- **`SendLatestBuilds`**: inviata quando si passa `--notify-latest`; invia un riepilogo dell'ultimo CU/GDR per versione SQL presente nel DB, indipendentemente dalla presenza di nuove righe.
- Entrambe le funzioni sono no-op se `email.enabled = false` o la lista dei destinatari è vuota.

### Deduplicazione

- **RawPages**: indice univoco su `(SourceKey, SHA256)`. Se la pagina non è cambiata, l'inserimento viene saltato (`IF NOT EXISTS`).
- **BuildRow**: `FindNewBuildRows` interroga il DB per i valori `KbNumber` già presenti prima di inserire; solo le nuove righe vengono accumulate per la notifica email.
- **KbPackageFiles**: `HasKbFiles` verifica la presenza di record per quel `KbNumber` prima di scaricare il CSV.

### Retry e throttling

Il client HTTP ritenta fino a **5 volte** con backoff esponenziale (`attempt² × 300ms`) in caso di errori di rete e risposte `429` / `5xx`. Il parametro `delayBetweenRequestsMs` introduce una pausa fissa tra ogni richiesta.

---

## Schema del database

### `dbo.RawPages`

Snapshot grezzo di ogni pagina scaricata.

| Colonna | Tipo | Note |
|---|---|---|
| `Id` | BIGINT IDENTITY | PK |
| `SourceKey` | NVARCHAR(200) | es. `sql2022-build-versions`, `kb-KB5078297` |
| `Url` | NVARCHAR(1000) | |
| `RetrievedAtUtc` | DATETIME2(3) | |
| `StatusCode` | INT | |
| `ETag` | NVARCHAR(200) | |
| `LastModified` | NVARCHAR(200) | |
| `ContentType` | NVARCHAR(200) | |
| `Sha256` | CHAR(64) | hash del body; indice univoco con SourceKey |
| `Html` | NVARCHAR(MAX) | body grezzo della risposta HTTP |

### `dbo.Sql{versione}_{topic}` (es. `dbo.Sql2022_CU_Builds`)

Una tabella per ogni combinazione `(versione SQL, tipo di aggiornamento)`. Topic possibili: `CU_Builds`, `GDR_Builds`, `AzureConnectPack_Builds`, `Other_Builds`.

| Colonna | Tipo |
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

Contenuto completo di ogni articolo KB.

| Colonna | Tipo |
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

File singoli inclusi in ogni pacchetto CU/GDR, estratti dal CSV pubblicato da Microsoft.

| Colonna | Tipo | Note |
|---|---|---|
| `Id` | BIGINT IDENTITY PK | |
| `KbNumber` | NVARCHAR(50) | FK logica → KbArticles |
| `Component` | NVARCHAR(200) | es. `SQL Server 2022 Database Engine` |
| `FileName` | NVARCHAR(500) | es. `sqlservr.exe` |
| `FileVersion` | NVARCHAR(50) | es. `2022.160.4236.2` |
| `FileSizeBytes` | BIGINT | |
| `FileDate` | DATE | data di build del file |
| `Platform` | NVARCHAR(20) | `x64`, `x86`, `n/a` |
| `RetrievedAtUtc` | DATETIME2(3) | |

---

## Dipendenze

| Libreria | Utilizzo |
|---|---|
| `github.com/PuerkitoBio/goquery` | Parsing HTML delle pagine Microsoft Learn |
| `github.com/denisenkom/go-mssqldb` | Driver SQL Server per `database/sql` |
| `github.com/spf13/pflag` | Parsing degli argomenti da riga di comando |
| `go.uber.org/zap` | Logging strutturato ad alte prestazioni |
| `gopkg.in/natefinch/lumberjack.v2` | Rotazione automatica dei file di log |
| `gopkg.in/yaml.v3` | Parsing file di configurazione YAML/JSON |
