package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"cu-watcher/internal/parse"
	"go.uber.org/zap"
)

// Inserisce con multi-row VALUES.
// Limite SQL Server: 2100 parametri. Qui: 13 colonne => max ~160 righe/batch.
func insertBuildRowsBatched(ctx context.Context, db *sql.DB, table string, rows []parse.BuildRow, log *zap.Logger) error {
	const colsPerRow = 13
	const maxParams = 2000 // margine
	maxRows := maxParams / colsPerRow
	if maxRows < 1 {
		maxRows = 1
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := 0; i < len(rows); i += maxRows {
		j := i + maxRows
		if j > len(rows) {
			j = len(rows)
		}
		if err := insertBatch(ctx, tx, table, rows[i:j]); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Info("inserted build rows", zap.String("table", table), zap.Int("count", len(rows)))
	return nil
}

func insertBatch(ctx context.Context, tx *sql.Tx, table string, batch []parse.BuildRow) error {
	var sb strings.Builder
	sb.WriteString("INSERT INTO dbo.")
	sb.WriteString(table)
	sb.WriteString(`(MajorVersion,Topic,UpdateName,SqlBuild,SqlFileVersion,AsBuild,AsFileVersion,KbNumber,KbUrl,ReleaseDate,ExtraJson,SourceUrl,RetrievedAtUtc) VALUES `)

	args := make([]any, 0, len(batch)*13)
	p := 1

	for idx, r := range batch {
		if idx > 0 {
			sb.WriteString(",")
		}
		// go-mssqldb supporta @p1... come in query parametrizzate
		sb.WriteString("(")
		for c := 0; c < 13; c++ {
			if c > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf("@p%d", p))
			p++
		}
		sb.WriteString(")")

		args = append(args,
			r.MajorVersion,
			r.Topic,
			nullS(r.UpdateName),
			nullS(r.SqlBuild),
			nullS(r.SqlFileVer),
			nullS(r.AsBuild),
			nullS(r.AsFileVer),
			nullS(r.KbNumber),
			nullS(r.KbURL),
			nullT(r.ReleaseDate),
			extraJSON(r.Extra),
			r.SourceURL,
			r.RetrievedAtUTC,
		)
	}

	_, err := tx.ExecContext(ctx, sb.String(), args...)
	return err
}

func nullS(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func nullT(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
