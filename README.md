# cu-watcher

Strumento Go che monitora e archivia i Cumulative Update (CU) e le patch GDR di SQL Server leggendo le pagine ufficiali Microsoft Learn, persistendo i dati su SQL Server e notificando via email le nuove release rilevate.

---

## Indice

- [Requisiti](#requisiti)
- [Installazione](#installazione)
- [Configurazione](#configurazione)
- [Utilizzo](#utilizzo)
- [Architettura](#architettura)
- [Schema del database](#schema-del-database)

---

## Requisiti

| Componente | Versione minima |
|---|---|
| Go | 1.24 |
| SQL Server | 2017+ |
| Connettività | accesso HTTPS a `learn.microsoft.com` e `download.microsoft.com` |

---

## Installazione

```bash
git clone <repo-url>
cd cu-watcher
go build -o cu-watcher ./cmd
```

---

## Configurazione

La configurazione è divisa in due file:

| File | Contenuto | Committare? |
|---|---|---|
| `config.yaml` | parametri non sensibili | ✅ sì |
| `.env` | credenziali e segreti | ❌ **mai** |

### 1. Creare il file .env

```bash
cp .env.example .env
```

Compilare i valori nel file `.env`:

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

Il file `.env` viene cercato nella stessa directory del file di configurazione YAML. Le variabili già presenti nell'ambiente di sistema hanno sempre la precedenza sul file `.env`.

### 2. Adattare config.yaml

```yaml
db:
  connectionString: "${DB_CONNECTION_STRING}"   # valore da .env

scraper:
  userAgent: "CUWatcher/1.0"
  timeoutSeconds: 60
  maxConcurrency: 4
  delayBetweenRequestsMs: 250
  followKbLinks: true        # scarica e analizza ogni articolo KB collegato
  maxKbToFetch: 500          # limite articoli KB per singola esecuzione
  since: "2017-01-01"        # esclude righe con ReleaseDate precedente

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
  enabled: false             # impostare true per abilitare le notifiche
  from: "${EMAIL_FROM}"
  to:
    - "${EMAIL_TO}"
  smtpHost: "${EMAIL_SMTP_HOST}"
  smtpPort: 465              # 587 = STARTTLS, 465 = TLS implicito
  username: "${EMAIL_USERNAME}"
  password: "${EMAIL_PASSWORD}"
  useTLS: true               # true per TLS implicito (porta 465)
```

### Parametri SMTP

| `smtpPort` | `useTLS` | Protocollo |
|---|---|---|
| 587 | `false` | STARTTLS |
| 465 | `true` | TLS implicito |

---

## Utilizzo

### Prima esecuzione — creazione tabelle

```bash
./cu-watcher --config config.yaml --init-db
```

### Esecuzione normale

```bash
./cu-watcher --config config.yaml
```

### Opzioni da riga di comando

Tutti i flag sovrascrivono il valore nel file di configurazione.

| Flag | Default | Descrizione |
|---|---|---|
| `--config` | `config.yaml` | Percorso del file di configurazione (yaml/json) |
| `--connection` | *(dal config)* | Override della connection string SQL Server |
| `--init-db` | `false` | Crea le tabelle base se non esistono |
| `--only` | *(tutte)* | Fonti da processare, es. `sql2022,sql2019` |
| `--follow-kb` | *(dal config)* | Override follow KB links (`true`/`false`) |
| `--max-kb` | *(dal config)* | Override numero massimo KB da scaricare |
| `--since` | *(dal config)* | Filtra righe con `ReleaseDate >= YYYY-MM-DD` |
| `--log-level` | *(dal config)* | Override livello di log |
| `--log-file` | *(dal config)* | Override percorso file di log |

### Esempi

```bash
# Processa solo SQL Server 2022, con log verboso
./cu-watcher --only sql2022 --log-level debug

# Forza il download dei soli articoli KB pubblicati dopo gennaio 2025
./cu-watcher --since 2025-01-01 --follow-kb=true --max-kb 100

# Usa una connection string diversa senza modificare il config
./cu-watcher --connection "sqlserver://sa:pass@localhost?database=test&trustservercertificate=true"
```

### Schedulazione (Linux/macOS)

```cron
# Esegue ogni giorno alle 06:00
0 6 * * * /opt/cu-watcher/cu-watcher --config /opt/cu-watcher/config.yaml >> /var/log/cu-watcher.log 2>&1
```

> Le variabili d'ambiente definite nel `.env` vengono caricate automaticamente. In alternativa è possibile esportarle nell'ambiente del processo (es. tramite systemd `EnvironmentFile=`).

---

## Architettura

```
cu-watcher/
├── .env.example                 # template variabili d'ambiente (da copiare in .env)
├── config.yaml                  # configurazione non sensibile
├── cmd/
│   └── main.go                  # entrypoint, orchestrazione del flusso
└── internal/
    ├── config/
    │   └── config.go            # Load(): carica .env, espande ${VAR}, parsing YAML
    ├── httpx/
    │   └── client.go            # HTTP client con retry (429/5xx), SHA256, logging
    ├── parse/
    │   ├── models.go            # BuildRow, KbArticle, KbFileRecord
    │   ├── build_versions.go    # parser pagine "build versions" di Microsoft Learn
    │   ├── kb_article.go        # parser articoli KB (titolo, sezioni, link, date)
    │   ├── kb_files.go          # estrazione link CSV e parsing file list per KB
    │   └── util.go              # helpers: clean, parseLearnDate, absURL, ...
    ├── db/
    │   ├── repo.go              # Repository: tutte le operazioni sul database
    │   ├── batch_insert.go      # INSERT multi-row batched per BuildRow
    │   ├── schema.go            # DDL delle tabelle base (baseSchemaSQL)
    │   └── sanitize.go          # sanitizzazione identificatori SQL
    ├── logging/
    │   └── logging.go           # zap logger (console + file rotante via lumberjack)
    └── notify/
        └── email.go             # notifica email SMTP per nuove release
```

### Flusso di esecuzione

```
main()
 │
 ├─ config.Load() → legge .env → espande ${VAR} → parsing YAML
 │
 ├─ Per ogni source (sql2022, sql2019, …)
 │   ├─ GET pagina "build-versions"
 │   ├─ SaveRawPage → dbo.RawPages (skip se SHA256 già presente)
 │   ├─ ParseBuildVersions → []BuildRow
 │   ├─ Per ogni topic table (CU_Builds, GDR_Builds, …)
 │   │   ├─ EnsureTopicTable → CREATE TABLE IF NOT EXISTS
 │   │   ├─ FindNewBuildRows → righe non ancora nel DB (dedup per KbNumber)
 │   │   └─ InsertBuildRowsBatched → dbo.Sql{version}_{topic}
 │   └─ ExtractKbLinks → kbQueue
 │
 ├─ Per ogni KB in kbQueue (fino a maxKbToFetch)
 │   ├─ GET articolo KB
 │   ├─ SaveRawPage → dbo.RawPages
 │   ├─ ParseKbArticle → KbArticle
 │   ├─ UpsertKbArticle → dbo.KbArticles
 │   ├─ HasKbFiles? → skip se già presenti
 │   ├─ ExtractKbFilesCSVLink → URL del CSV file list
 │   ├─ GET CSV da download.microsoft.com
 │   ├─ ParseKbFilesCSV → []KbFileRecord (strip BOM UTF-8)
 │   └─ InsertKbFileRecords → dbo.KbPackageFiles
 │
 └─ SendNewReleases → email SMTP (solo se enabled e ci sono nuove righe)
```

### Gestione segreti

```
.env (non committato)          config.yaml (committato)
─────────────────────          ────────────────────────
DB_CONNECTION_STRING=…    →    connectionString: "${DB_CONNECTION_STRING}"
EMAIL_USERNAME=…          →    username: "${EMAIL_USERNAME}"
EMAIL_PASSWORD=…          →    password: "${EMAIL_PASSWORD}"
EMAIL_FROM=…              →    from: "${EMAIL_FROM}"
EMAIL_TO=…                →    to: ["${EMAIL_TO}"]
EMAIL_SMTP_HOST=…         →    smtpHost: "${EMAIL_SMTP_HOST}"
```

Priorità di risoluzione: **env di sistema** > `.env` > valore letterale in `config.yaml`.

### Deduplicazione

- **RawPages**: indice unico su `(SourceKey, SHA256)`. Se la pagina non è cambiata l'insert viene saltato (`IF NOT EXISTS`).
- **BuildRow**: `FindNewBuildRows` interroga il DB per i `KbNumber` già presenti prima di inserire; solo le righe nuove vengono accumulate per la notifica email.
- **KbPackageFiles**: `HasKbFiles` verifica l'esistenza di record per quel `KbNumber` prima di scaricare il CSV.

### Retry e throttling

L'HTTP client esegue fino a **5 tentativi** con backoff esponenziale (`attempt² × 300ms`) su errori di rete e risposte `429` / `5xx`. Il parametro `delayBetweenRequestsMs` introduce una pausa fissa tra ogni richiesta.

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
| `Sha256` | CHAR(64) | hash del body; indice unico con SourceKey |
| `Html` | NVARCHAR(MAX) | body grezzo della risposta |

### `dbo.Sql{version}_{topic}` (es. `dbo.Sql2022_CU_Builds`)

Una tabella per combinazione `(versione SQL, tipo aggiornamento)`. I topic possibili sono: `CU_Builds`, `GDR_Builds`, `AzureConnectPack_Builds`, `Other_Builds`.

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

File individuali inclusi in ogni pacchetto CU/GDR, estratti dal CSV pubblicato da Microsoft.

| Colonna | Tipo | Note |
|---|---|---|
| `Id` | BIGINT IDENTITY PK | |
| `KbNumber` | NVARCHAR(50) | FK logico → KbArticles |
| `Component` | NVARCHAR(200) | es. `SQL Server 2022 Database Engine` |
| `FileName` | NVARCHAR(500) | es. `sqlservr.exe` |
| `FileVersion` | NVARCHAR(50) | es. `2022.160.4236.2` |
| `FileSizeBytes` | BIGINT | |
| `FileDate` | DATE | data di compilazione del file |
| `Platform` | NVARCHAR(20) | `x64`, `x86`, `n/a` |
| `RetrievedAtUtc` | DATETIME2(3) | |

---

## Dipendenze principali

| Libreria | Utilizzo |
|---|---|
| `github.com/PuerkitoBio/goquery` | parsing HTML delle pagine Microsoft Learn |
| `github.com/denisenkom/go-mssqldb` | driver SQL Server per `database/sql` |
| `github.com/spf13/pflag` | parsing argomenti CLI |
| `go.uber.org/zap` | logging strutturato ad alte prestazioni |
| `gopkg.in/natefinch/lumberjack.v2` | rotazione automatica dei file di log |
| `gopkg.in/yaml.v3` | parsing file di configurazione YAML/JSON |
