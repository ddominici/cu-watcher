package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"cu-watcher/internal/config"
	"cu-watcher/internal/db"
	"cu-watcher/internal/httpx"
	"cu-watcher/internal/logging"
	"cu-watcher/internal/notify"
	"cu-watcher/internal/parse"
)

type cliOptions struct {
	ConfigPath  string
	Connection  string
	InitDB      bool
	Only        string
	FollowKB    *bool
	MaxKB       int
	Since       string
	LogLevel    string
	LogFile     string
	NotifyLatest bool
	WindowsAuth bool
}

func main() {
	var opt cliOptions

	pflag.StringVar(&opt.ConfigPath, "config", "config.yaml", "Path to config file (yaml/json).")
	pflag.StringVar(&opt.Connection, "connection", "", "Override SQL Server connection string.")
	pflag.BoolVar(&opt.InitDB, "init-db", false, "Create base tables if missing.")
	pflag.StringVar(&opt.Only, "only", "", "Comma-separated source keys (e.g. sql2022,sql2019).")
	pflag.IntVar(&opt.MaxKB, "max-kb", 0, "Override max KB pages to fetch (0 = config).")
	pflag.StringVar(&opt.Since, "since", "", "Only persist rows with ReleaseDate >= since (YYYY-MM-DD).")
	pflag.StringVar(&opt.LogLevel, "log-level", "", "Override log level (debug/info/warn/error).")
	pflag.StringVar(&opt.LogFile, "log-file", "", "Override log file path.")
	pflag.BoolVar(&opt.NotifyLatest, "notify-latest", false, "Send email with the latest CU/GDR per SQL Server version (from DB).")
	pflag.BoolVar(&opt.WindowsAuth, "windows-auth", false, "Use Windows Authentication (IntegratedSecurity=true). Overrides config.")
	{
		var v string
		pflag.StringVar(&v, "follow-kb", "", "Override follow KB links (true/false). Empty uses config.")
		pflag.Lookup("follow-kb").NoOptDefVal = "true"
		pflag.CommandLine.AddGoFlagSet(nil)
		pflag.Parse()

		if v != "" {
			b := strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
			opt.FollowKB = &b
		}
	}

	cfg, err := config.Load(opt.ConfigPath)
	exitOnErr(err)

	if opt.Connection != "" {
		cfg.DB.ConnectionString = opt.Connection
	}
	if opt.LogLevel != "" {
		cfg.Logging.Level = opt.LogLevel
	}
	if opt.LogFile != "" {
		cfg.Logging.File = opt.LogFile
	}
	if opt.FollowKB != nil {
		cfg.Scraper.FollowKBLinks = *opt.FollowKB
	}
	if opt.MaxKB > 0 {
		cfg.Scraper.MaxKBToFetch = opt.MaxKB
	}
	if opt.Since != "" {
		cfg.Scraper.Since = opt.Since
	}
	if opt.WindowsAuth {
		cfg.DB.WindowsAuth = true
	}
	if cfg.DB.WindowsAuth {
		cfg.DB.ConnectionString = withIntegratedSecurity(cfg.DB.ConnectionString)
	}

	log := logging.New(logging.Config{
		Level:      cfg.Logging.Level,
		File:       cfg.Logging.File,
		MaxSizeMB:  cfg.Logging.MaxSizeMB,
		MaxBackups: cfg.Logging.MaxBackups,
		MaxAgeDays: cfg.Logging.MaxAgeDays,
	})
	defer func() { _ = log.Sync() }()

	if cfg.DB.ConnectionString == "" {
		exitOnErr(errors.New("missing db.connectionString (or --connection)"))
	}

	ctx := context.Background()

	repo, err := db.NewRepository(cfg.DB.ConnectionString, log)
	exitOnErr(err)
	defer repo.Close()

	if opt.InitDB {
		exitOnErr(repo.InitSchema(ctx))
	}

	client := httpx.New(httpx.ScraperCfg{
		UserAgent:              cfg.Scraper.UserAgent,
		TimeoutSeconds:         cfg.Scraper.TimeoutSeconds,
		DelayBetweenRequestsMs: cfg.Scraper.DelayBetweenRequestsMs,
	}, log)

	onlySet := map[string]bool{}
	if strings.TrimSpace(opt.Only) != "" {
		for _, k := range strings.Split(opt.Only, ",") {
			k = strings.TrimSpace(k)
			if k != "" {
				onlySet[strings.ToLower(k)] = true
			}
		}
	}

	var sinceDate *time.Time
	if cfg.Scraper.Since != "" {
		t, e := time.Parse("2006-01-02", cfg.Scraper.Since)
		exitOnErr(e)
		sinceDate = &t
	}

	runID := hex.EncodeToString(sha256.New().Sum([]byte(fmt.Sprintf("%d", time.Now().UnixNano()))))[:12]
	log.Info("run start", logging.F("runId", runID))

	var newReleases []parse.BuildRow // accumulates truly-new rows for email notification

	kbQueue := make(map[string]string) // kb -> url

	for _, src := range cfg.Sources {
		if len(onlySet) > 0 && !onlySet[strings.ToLower(src.Key)] {
			continue
		}

		page, err := client.Get(ctx, src.Key+"-build-versions", src.URL)
		if err != nil {
			log.Error("fetch failed", logging.F("url", src.URL), logging.E(err))
			continue
		}

		exitOnErr(repo.SaveRawPage(ctx, page))

		rows, err := parse.ParseBuildVersions(src.MajorVersion, src.URL, page.HTML, page.RetrievedAtUTC)
		if err != nil {
			log.Error("parse build versions failed", logging.F("url", src.URL), logging.E(err))
			continue
		}

		// filter since
		if sinceDate != nil {
			filtered := rows[:0]
			for _, r := range rows {
				if r.ReleaseDate == nil || !r.ReleaseDate.Before(*sinceDate) {
					filtered = append(filtered, r)
				}
			}
			rows = filtered
		}

		// group by topic table
		groups := parse.GroupByTopicTable(rows)

		for table, g := range groups {
			exitOnErr(repo.EnsureTopicTable(ctx, table))
			fresh, err := repo.FindNewBuildRows(ctx, table, g)
			exitOnErr(err)
			exitOnErr(repo.InsertBuildRowsBatched(ctx, table, g))
			newReleases = append(newReleases, fresh...)
		}

		if cfg.Scraper.FollowKBLinks {
			links := parse.ExtractKbLinks(page.HTML, src.URL)
			for kb, url := range links {
				if _, ok := kbQueue[kb]; !ok {
					kbQueue[kb] = url
				}
			}
		}
	}

	if cfg.Scraper.FollowKBLinks {
		limit := cfg.Scraper.MaxKBToFetch
		if limit <= 0 || limit > len(kbQueue) {
			limit = len(kbQueue)
		}

		i := 0
		for kb, url := range kbQueue {
			if i >= limit {
				break
			}
			i++

			page, err := client.Get(ctx, "kb-"+kb, url)
			if err != nil {
				log.Error("fetch KB failed", logging.F("kb", kb), logging.F("url", url), logging.E(err))
				continue
			}
			exitOnErr(repo.SaveRawPage(ctx, page))

			art, err := parse.ParseKbArticle(kb, url, page.HTML, page.RetrievedAtUTC)
			if err != nil {
				log.Error("parse KB failed", logging.F("kb", kb), logging.E(err))
				continue
			}
			exitOnErr(repo.UpsertKbArticle(ctx, art))

			// Download and store the per-KB file list (skip if already loaded).
			// SQL 2022/2025: separate CSV download linked from the article.
			// SQL 2019/2017: file tables embedded directly in the HTML.
			hasFiles, err := repo.HasKbFiles(ctx, kb)
			exitOnErr(err)
			if !hasFiles {
				var records []parse.KbFileRecord

				csvURL := parse.ExtractKbFilesCSVLink(page.HTML, url)
				if csvURL != "" {
					csvPage, err := client.Get(ctx, "kb-files-"+kb, csvURL)
					if err != nil {
						log.Error("fetch KB file list CSV failed", logging.F("kb", kb), logging.F("url", csvURL), logging.E(err))
					} else {
						records = parse.ParseKbFilesCSV(kb, csvPage.HTML, csvPage.RetrievedAtUTC)
					}
				} else {
					records = parse.ParseKbFilesHTML(kb, page.HTML, page.RetrievedAtUTC)
				}

				if len(records) > 0 {
					if err := repo.InsertKbFileRecords(ctx, records); err != nil {
						log.Error("save KB file records failed", logging.F("kb", kb), logging.E(err))
					} else {
						log.Info("KB file records saved", logging.F("kb", kb), logging.F("files", len(records)))
					}
				}
			}
		}
	}

	log.Info("run done", logging.F("runId", runID), logging.F("newReleases", len(newReleases)))

	emailCfg := notify.EmailConfig{
		Enabled:  cfg.Email.Enabled,
		From:     cfg.Email.From,
		To:       cfg.Email.To,
		SMTPHost: cfg.Email.SMTPHost,
		SMTPPort: cfg.Email.SMTPPort,
		Username: cfg.Email.Username,
		Password: cfg.Email.Password,
		UseTLS:   cfg.Email.UseTLS,
	}
	if err := notify.SendNewReleases(emailCfg, newReleases); err != nil {
		log.Error("failed to send notification email", logging.E(err))
	}

	if opt.NotifyLatest {
		latest, err := repo.GetLatestBuilds(ctx)
		if err != nil {
			log.Error("failed to query latest builds", logging.E(err))
		} else if err := notify.SendLatestBuilds(emailCfg, latest); err != nil {
			log.Error("failed to send latest-builds notification email", logging.E(err))
		}
	}
}

func exitOnErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// withIntegratedSecurity appends IntegratedSecurity=true to conn if it is not
// already present. Supports both the URL form (sqlserver://...) and the
// semicolon-separated DSN form.
func withIntegratedSecurity(conn string) string {
	lower := strings.ToLower(conn)
	if strings.Contains(lower, "integratedsecurity") || strings.Contains(lower, "integrated security") {
		return conn
	}
	if strings.HasPrefix(conn, "sqlserver://") {
		if strings.Contains(conn, "?") {
			return conn + "&IntegratedSecurity=true"
		}
		return conn + "?IntegratedSecurity=true"
	}
	// DSN / key=value format
	return conn + ";IntegratedSecurity=true"
}
