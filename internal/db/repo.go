package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/denisenkom/go-mssqldb"
	"go.uber.org/zap"

	"cu-watcher/internal/httpx"
	"cu-watcher/internal/parse"
)

type Repository struct {
	db  *sql.DB
	log *zap.Logger
}

func NewRepository(conn string, log *zap.Logger) (*Repository, error) {
	db, err := sql.Open("sqlserver", conn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Repository{db: db, log: log}, nil
}

func (r *Repository) Close() { _ = r.db.Close() }

func (r *Repository) InitSchema(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, baseSchemaSQL)
	return err
}

func (r *Repository) SaveRawPage(ctx context.Context, p *httpx.RawPage) error {
	// Skip insert when the exact same content (SourceKey+SHA256) already exists.
	const q = `
IF NOT EXISTS (SELECT 1 FROM dbo.RawPages WHERE SourceKey=@p1 AND Sha256=@p9)
  INSERT INTO dbo.RawPages(SourceKey, Url, RetrievedAtUtc, StatusCode, ETag, LastModified, ContentType, Sha256, Html)
  VALUES(@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9);
`
	_, err := r.db.ExecContext(ctx, q,
		p.SourceKey, p.URL, p.RetrievedAtUTC, p.StatusCode,
		nullStr(p.ETag), nullStr(p.LastModified), nullStr(p.ContentType),
		p.SHA256, p.HTML,
	)
	return err
}

func (r *Repository) EnsureTopicTable(ctx context.Context, table string) error {
	t := SanitizeIdentifier(table)
	q := fmt.Sprintf(`
IF OBJECT_ID('dbo.%s') IS NULL
BEGIN
  CREATE TABLE dbo.%s(
    Id BIGINT IDENTITY(1,1) PRIMARY KEY,
    MajorVersion INT NOT NULL,
    Topic NVARCHAR(200) NOT NULL,
    UpdateName NVARCHAR(100) NULL,
    SqlBuild NVARCHAR(50) NULL,
    SqlFileVersion NVARCHAR(50) NULL,
    AsBuild NVARCHAR(50) NULL,
    AsFileVersion NVARCHAR(50) NULL,
    KbNumber NVARCHAR(50) NULL,
    KbUrl NVARCHAR(1000) NULL,
    ReleaseDate DATE NULL,
    ExtraJson NVARCHAR(MAX) NULL,
    SourceUrl NVARCHAR(1000) NOT NULL,
    RetrievedAtUtc DATETIME2(3) NOT NULL
  );
  CREATE INDEX IX_%s_KbNumber ON dbo.%s(KbNumber);
END;
`, t, t, t, t)
	_, err := r.db.ExecContext(ctx, q)
	return err
}

// FindNewBuildRows returns the subset of rows whose KbNumber is not yet present
// in the given topic table. Rows without a KbNumber are excluded from the result
// (their identity is ambiguous for notification purposes).
func (r *Repository) FindNewBuildRows(ctx context.Context, table string, rows []parse.BuildRow) ([]parse.BuildRow, error) {
	// Collect distinct non-empty KbNumbers from candidates.
	seen := map[string]bool{}
	kbs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.KbNumber != "" && !seen[row.KbNumber] {
			seen[row.KbNumber] = true
			kbs = append(kbs, row.KbNumber)
		}
	}
	if len(kbs) == 0 {
		return nil, nil
	}

	t := SanitizeIdentifier(table)
	placeholders := make([]string, len(kbs))
	args := make([]any, len(kbs))
	for i, kb := range kbs {
		placeholders[i] = fmt.Sprintf("@p%d", i+1)
		args[i] = kb
	}
	q := fmt.Sprintf("SELECT DISTINCT KbNumber FROM dbo.%s WHERE KbNumber IN (%s)", t, strings.Join(placeholders, ","))

	rws, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rws.Close()

	existing := map[string]bool{}
	for rws.Next() {
		var kb string
		if err := rws.Scan(&kb); err != nil {
			return nil, err
		}
		existing[kb] = true
	}
	if err := rws.Err(); err != nil {
		return nil, err
	}

	var newRows []parse.BuildRow
	for _, row := range rows {
		if row.KbNumber != "" && !existing[row.KbNumber] {
			newRows = append(newRows, row)
		}
	}
	return newRows, nil
}

func (r *Repository) InsertBuildRowsBatched(ctx context.Context, table string, rows []parse.BuildRow) error {
	t := SanitizeIdentifier(table)
	return insertBuildRowsBatched(ctx, r.db, t, rows, r.log)
}

func (r *Repository) UpsertKbArticle(ctx context.Context, a parse.KbArticle) error {
	const q = `
MERGE dbo.KbArticles AS t
USING (SELECT @p1 AS KbNumber) AS s
ON (t.KbNumber = s.KbNumber)
WHEN MATCHED THEN
  UPDATE SET Url=@p2, Title=@p3, AppliesTo=@p4, ReleaseDate=@p5, ProductVersion=@p6,
             RetrievedAtUtc=@p7, ContentText=@p8, ContentHtml=@p9, SectionsJson=@p10, ExtraJson=@p11
WHEN NOT MATCHED THEN
  INSERT (KbNumber, Url, Title, AppliesTo, ReleaseDate, ProductVersion, RetrievedAtUtc, ContentText, ContentHtml, SectionsJson, ExtraJson)
  VALUES (@p1,@p2,@p3,@p4,@p5,@p6,@p7,@p8,@p9,@p10,@p11);
`
	_, err := r.db.ExecContext(ctx, q,
		a.KbNumber, a.URL, nullStr(a.Title), nullStr(a.AppliesTo),
		nullTime(a.ReleaseDate), nullStr(a.ProductVersion),
		a.RetrievedAtUTC, nullStr(a.ContentText), nullStr(a.ContentHTML),
		nullStr(a.SectionsJSON), nullStr(a.ExtraJSON),
	)
	return err
}

// HasKbFiles returns true if file records for this KbNumber already exist.
func (r *Repository) HasKbFiles(ctx context.Context, kbNumber string) (bool, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		"SELECT COUNT(1) FROM dbo.KbPackageFiles WHERE KbNumber=@p1", kbNumber,
	).Scan(&n)
	return n > 0, err
}

// InsertKbFileRecords bulk-inserts file records for a KB package.
func (r *Repository) InsertKbFileRecords(ctx context.Context, records []parse.KbFileRecord) error {
	if len(records) == 0 {
		return nil
	}
	const colsPerRow = 8
	const maxParams = 2000
	maxRows := maxParams / colsPerRow

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for i := 0; i < len(records); i += maxRows {
		j := i + maxRows
		if j > len(records) {
			j = len(records)
		}
		if err := insertKbFilesBatch(ctx, tx, records[i:j]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func insertKbFilesBatch(ctx context.Context, tx *sql.Tx, batch []parse.KbFileRecord) error {
	var sb strings.Builder
	sb.WriteString(`INSERT INTO dbo.KbPackageFiles` +
		`(KbNumber,Component,FileName,FileVersion,FileSizeBytes,FileDate,Platform,RetrievedAtUtc) VALUES `)

	args := make([]any, 0, len(batch)*8)
	p := 1
	for idx, rec := range batch {
		if idx > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf("(@p%d,@p%d,@p%d,@p%d,@p%d,@p%d,@p%d,@p%d)",
			p, p+1, p+2, p+3, p+4, p+5, p+6, p+7))
		p += 8

		var sizeArg any
		if rec.FileSizeBytes > 0 {
			sizeArg = rec.FileSizeBytes
		}
		args = append(args,
			rec.KbNumber,
			nullStr(rec.Component),
			rec.FileName,
			nullStr(rec.FileVersion),
			sizeArg,
			nullTime(rec.FileDate),
			nullStr(rec.Platform),
			rec.RetrievedAtUTC,
		)
	}
	_, err := tx.ExecContext(ctx, sb.String(), args...)
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}

func extraJSON(m map[string]string) any {
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	return string(b)
}
