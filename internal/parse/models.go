package parse

import "time"

type BuildRow struct {
	MajorVersion   int
	Topic          string
	UpdateName     string
	SqlBuild       string
	SqlFileVer     string
	AsBuild        string
	AsFileVer      string
	KbNumber       string
	KbURL          string
	ReleaseDate    *time.Time
	SourceURL      string
	RetrievedAtUTC time.Time
	Extra          map[string]string
}

type KbFileRecord struct {
	KbNumber      string
	Component     string
	FileName      string
	FileVersion   string
	FileSizeBytes int64
	FileDate      *time.Time
	Platform      string
	RetrievedAtUTC time.Time
}

type KbArticle struct {
	KbNumber       string
	URL            string
	Title          string
	AppliesTo      string
	ReleaseDate    *time.Time
	ProductVersion string
	RetrievedAtUTC time.Time
	ContentText    string
	ContentHTML    string
	SectionsJSON   string
	ExtraJSON      string
}
