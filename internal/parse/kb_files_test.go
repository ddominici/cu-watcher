package parse

import (
	"testing"
	"time"
)

// csvSample riproduce fedelmente la struttura reale dei CSV Microsoft:
//   - BOM UTF-8 in testa (ef bb bf)
//   - titolo sezione (col[1] vuoto)
//   - riga header (skippata)
//   - righe file
//   - riga separatore vuota tra sezioni
//   - seconda sezione
const csvSample = "\xef\xbb\xbf" + // UTF-8 BOM
	"SQL Server 2022 Analysis Services,,,,,,,,,,,,,,,\r\n" +
	"File name,File version,File size,Date,Time,Platform,,,,,,,,,,\r\n" +
	"asplatformhost.dll,2022.160.43.252,336928,22-Jan-26,20:24,x64,,,,,,,,,,\r\n" +
	"mashupsql.config,n/a,449,22-Jan-26,20:24,n/a,,,,,,,,,,\r\n" +
	"anglesharp.dll,0.9.9.0,1240112,22-Jan-26,20:24,x86,,,,,,,,,,\r\n" +
	",,,,,,,,,,,,,,,\r\n" + // riga separatore vuota
	"SQL Server 2022 Database Services Core Instance,,,,,,,,,,,,,,,\r\n" +
	"File name,File version,File size,Date,Time,Platform,,,,,,,,,,\r\n" +
	"sqlservr.exe,2022.160.4236.2,313448,22-Jan-26,20:24,x64,,,,,,,,,,\r\n" +
	"sqlos.dll,2022.160.4236.2,104488,22-Jan-26,20:24,x64,,,,,,,,,,\r\n"

func TestParseKbFilesCSV_BOMStripped(t *testing.T) {
	kb := "KB5078297"
	retrieved := time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC)

	records := ParseKbFilesCSV(kb, csvSample, retrieved)

	if len(records) == 0 {
		t.Fatal("nessun record prodotto")
	}

	// Il primo componente NON deve avere il prefisso BOM
	first := records[0]
	want := "SQL Server 2022 Analysis Services"
	if first.Component != want {
		t.Errorf("componente con BOM: got %q, want %q", first.Component, want)
	}
}

func TestParseKbFilesCSV_RecordCount(t *testing.T) {
	records := ParseKbFilesCSV("KB5078297", csvSample, time.Now())
	// 3 file nella prima sezione + 2 nella seconda = 5 totali
	if len(records) != 5 {
		t.Errorf("attesi 5 record, ottenuti %d", len(records))
		for i, r := range records {
			t.Logf("  [%d] component=%q file=%q", i, r.Component, r.FileName)
		}
	}
}

func TestParseKbFilesCSV_HeaderSkipped(t *testing.T) {
	records := ParseKbFilesCSV("KB5078297", csvSample, time.Now())
	for _, r := range records {
		if r.FileName == "File name" {
			t.Error("la riga header non deve essere inclusa nei record")
		}
	}
}

func TestParseKbFilesCSV_FieldValues(t *testing.T) {
	retrieved := time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC)
	records := ParseKbFilesCSV("KB5078297", csvSample, retrieved)

	// Primo file della prima sezione
	r := records[0]
	if r.KbNumber != "KB5078297" {
		t.Errorf("KbNumber: got %q, want %q", r.KbNumber, "KB5078297")
	}
	if r.FileName != "asplatformhost.dll" {
		t.Errorf("FileName: got %q, want %q", r.FileName, "asplatformhost.dll")
	}
	if r.FileVersion != "2022.160.43.252" {
		t.Errorf("FileVersion: got %q, want %q", r.FileVersion, "2022.160.43.252")
	}
	if r.FileSizeBytes != 336928 {
		t.Errorf("FileSizeBytes: got %d, want %d", r.FileSizeBytes, 336928)
	}
	if r.Platform != "x64" {
		t.Errorf("Platform: got %q, want %q", r.Platform, "x64")
	}
	if r.RetrievedAtUTC != retrieved {
		t.Errorf("RetrievedAtUTC: got %v, want %v", r.RetrievedAtUTC, retrieved)
	}
}

func TestParseKbFilesCSV_FileDate(t *testing.T) {
	records := ParseKbFilesCSV("KB5078297", csvSample, time.Now())
	r := records[0]
	if r.FileDate == nil {
		t.Fatal("FileDate è nil, attesa 2026-01-22")
	}
	want := time.Date(2026, 1, 22, 0, 0, 0, 0, time.UTC)
	if !r.FileDate.Equal(want) {
		t.Errorf("FileDate: got %v, want %v", r.FileDate, want)
	}
}

func TestParseKbFilesCSV_NaValues(t *testing.T) {
	records := ParseKbFilesCSV("KB5078297", csvSample, time.Now())
	// mashupsql.config ha version="n/a" e platform="n/a"
	var cfg *KbFileRecord
	for i := range records {
		if records[i].FileName == "mashupsql.config" {
			cfg = &records[i]
			break
		}
	}
	if cfg == nil {
		t.Fatal("mashupsql.config non trovato nei record")
	}
	if cfg.FileVersion != "n/a" {
		t.Errorf("FileVersion: got %q, want %q", cfg.FileVersion, "n/a")
	}
	if cfg.Platform != "n/a" {
		t.Errorf("Platform: got %q, want %q", cfg.Platform, "n/a")
	}
	if cfg.FileSizeBytes != 449 {
		t.Errorf("FileSizeBytes: got %d, want %d", cfg.FileSizeBytes, 449)
	}
}

func TestParseKbFilesCSV_MultiSection(t *testing.T) {
	records := ParseKbFilesCSV("KB5078297", csvSample, time.Now())

	// I primi 3 record devono essere in "SQL Server 2022 Analysis Services"
	for i := 0; i < 3; i++ {
		want := "SQL Server 2022 Analysis Services"
		if records[i].Component != want {
			t.Errorf("record[%d].Component = %q, want %q", i, records[i].Component, want)
		}
	}
	// Gli ultimi 2 record devono essere in "SQL Server 2022 Database Services Core Instance"
	for i := 3; i < 5; i++ {
		want := "SQL Server 2022 Database Services Core Instance"
		if records[i].Component != want {
			t.Errorf("record[%d].Component = %q, want %q", i, records[i].Component, want)
		}
	}
}

func TestParseKbFilesCSV_EmptyInput(t *testing.T) {
	records := ParseKbFilesCSV("KB0000000", "", time.Now())
	if len(records) != 0 {
		t.Errorf("input vuoto: attesi 0 record, ottenuti %d", len(records))
	}
}

func TestParseKbFilesCSV_OnlyBOM(t *testing.T) {
	records := ParseKbFilesCSV("KB0000000", "\xef\xbb\xbf", time.Now())
	if len(records) != 0 {
		t.Errorf("solo BOM: attesi 0 record, ottenuti %d", len(records))
	}
}

func TestExtractKbFilesCSVLink_Found(t *testing.T) {
	html := `<html><body>
<p>Download <a href="https://download.microsoft.com/download/abc/KB5078297.csv">the list of files that are included in KB5078297</a>.</p>
</body></html>`

	got := ExtractKbFilesCSVLink(html, "https://learn.microsoft.com/en-us/troubleshoot/sql/releases/sqlserver-2022/cumulativeupdate23")
	want := "https://download.microsoft.com/download/abc/KB5078297.csv"
	if got != want {
		t.Errorf("ExtractKbFilesCSVLink: got %q, want %q", got, want)
	}
}

func TestExtractKbFilesCSVLink_NotFound(t *testing.T) {
	html := `<html><body><p>No CSV link here.</p></body></html>`
	got := ExtractKbFilesCSVLink(html, "https://learn.microsoft.com/")
	if got != "" {
		t.Errorf("atteso stringa vuota, ottenuto %q", got)
	}
}

func TestExtractKbFilesCSVLink_RelativeURL(t *testing.T) {
	html := `<html><body>
<a href="/download/abc/KB1234567.csv">file list</a>
</body></html>`
	base := "https://learn.microsoft.com/en-us/troubleshoot/sql/"
	got := ExtractKbFilesCSVLink(html, base)
	want := "https://learn.microsoft.com/download/abc/KB1234567.csv"
	if got != want {
		t.Errorf("URL relativo: got %q, want %q", got, want)
	}
}

func TestExtractKbFilesCSVLink_IgnoresNonCSV(t *testing.T) {
	html := `<html><body>
<a href="https://example.com/doc.pdf">pdf</a>
<a href="https://example.com/data.xlsx">excel</a>
<a href="https://download.microsoft.com/KB.csv">csv</a>
</body></html>`
	got := ExtractKbFilesCSVLink(html, "https://learn.microsoft.com/")
	want := "https://download.microsoft.com/KB.csv"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// htmlSample riproduce fedelmente la struttura HTML reale delle pagine KB
// di SQL Server 2019/2017: tabelle file dentro un <details> con <summary>
// "Cumulative Update package file information". I nomi dei componenti sono
// tag <p> che precedono ogni <table>.
const htmlSample = `<html><body>
<h2 id="file-information">File information</h2>
<p>You can verify the download by computing the hash of the <em>SQLServer2019-KB5054833-x64.exe</em> file.</p>
<table>
  <thead><tr><th>File name</th><th>SHA256 hash</th></tr></thead>
  <tbody><tr><td>SQLServer2019-KB5054833-x64.exe</td><td>864AA0185…</td></tr></tbody>
</table>
<details>
  <summary><b>Cumulative Update package file information</b></summary>
  <p>The English version of this package has the file attributes listed in the following table.</p>
  <p>x64-based versions</p>
  <p>SQL Server 2019 Analysis Services</p>
  <table>
    <thead><tr><th>File name</th><th>File version</th><th>File size</th><th>Date</th><th>Time</th><th>Platform</th></tr></thead>
    <tbody>
      <tr><td>Asplatformhost.dll</td><td>2018.150.35.51</td><td>292912</td><td>21-Feb-2025</td><td>18:02</td><td>x64</td></tr>
      <tr><td>Mashupcompression.dll</td><td>2.87.142.0</td><td>140672</td><td>21-Feb-2025</td><td>18:06</td><td>x64</td></tr>
    </tbody>
  </table>
  <p>SQL Server 2019 Database Services Core Instance</p>
  <table>
    <thead><tr><th>File name</th><th>File version</th><th>File size</th><th>Date</th><th>Time</th><th>Platform</th></tr></thead>
    <tbody>
      <tr><td>Sqlservr.exe</td><td>2019.150.4430.1</td><td>6634552</td><td>21-Feb-2025</td><td>18:06</td><td>x64</td></tr>
    </tbody>
  </table>
</details>
</body></html>`

func TestParseKbFilesHTML_RecordCount(t *testing.T) {
	records := ParseKbFilesHTML("KB5054833", htmlSample, time.Now())
	// 2 file Analysis Services + 1 Database Services = 3
	if len(records) != 3 {
		t.Errorf("attesi 3 record, ottenuti %d", len(records))
		for i, r := range records {
			t.Logf("  [%d] component=%q file=%q", i, r.Component, r.FileName)
		}
	}
}

func TestParseKbFilesHTML_SkipsHashTable(t *testing.T) {
	records := ParseKbFilesHTML("KB5054833", htmlSample, time.Now())
	for _, r := range records {
		if r.FileName == "SQLServer2019-KB5054833-x64.exe" {
			t.Error("la tabella SHA256 non deve essere inclusa")
		}
	}
}

func TestParseKbFilesHTML_SkipsNonComponentParagraphs(t *testing.T) {
	records := ParseKbFilesHTML("KB5054833", htmlSample, time.Now())
	for _, r := range records {
		if r.Component == "x64-based versions" || r.Component == "" {
			t.Errorf("componente non valido: %q per file %q", r.Component, r.FileName)
		}
	}
}

func TestParseKbFilesHTML_MultiSection(t *testing.T) {
	records := ParseKbFilesHTML("KB5054833", htmlSample, time.Now())
	if len(records) < 3 {
		t.Fatal("record insufficienti per verificare multi-sezione")
	}
	// I primi 2 devono essere in Analysis Services
	for i := 0; i < 2; i++ {
		want := "SQL Server 2019 Analysis Services"
		if records[i].Component != want {
			t.Errorf("record[%d].Component = %q, want %q", i, records[i].Component, want)
		}
	}
	// Il terzo deve essere in Database Services Core Instance
	want := "SQL Server 2019 Database Services Core Instance"
	if records[2].Component != want {
		t.Errorf("record[2].Component = %q, want %q", records[2].Component, want)
	}
}

func TestParseKbFilesHTML_FieldValues(t *testing.T) {
	retrieved := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	records := ParseKbFilesHTML("KB5054833", htmlSample, retrieved)
	if len(records) == 0 {
		t.Fatal("nessun record")
	}
	r := records[0]
	if r.KbNumber != "KB5054833" {
		t.Errorf("KbNumber: got %q, want %q", r.KbNumber, "KB5054833")
	}
	if r.FileName != "Asplatformhost.dll" {
		t.Errorf("FileName: got %q, want %q", r.FileName, "Asplatformhost.dll")
	}
	if r.FileVersion != "2018.150.35.51" {
		t.Errorf("FileVersion: got %q, want %q", r.FileVersion, "2018.150.35.51")
	}
	if r.FileSizeBytes != 292912 {
		t.Errorf("FileSizeBytes: got %d, want %d", r.FileSizeBytes, 292912)
	}
	if r.Platform != "x64" {
		t.Errorf("Platform: got %q, want %q", r.Platform, "x64")
	}
}

func TestParseKbFilesHTML_DateFormat4DigitYear(t *testing.T) {
	records := ParseKbFilesHTML("KB5054833", htmlSample, time.Now())
	if len(records) == 0 {
		t.Fatal("nessun record")
	}
	r := records[0]
	if r.FileDate == nil {
		t.Fatal("FileDate è nil, attesa 2025-02-21")
	}
	want := time.Date(2025, time.February, 21, 0, 0, 0, 0, time.UTC)
	if !r.FileDate.Equal(want) {
		t.Errorf("FileDate: got %v, want %v", r.FileDate, want)
	}
}

func TestParseKbFilesHTML_NoDetailsBlock(t *testing.T) {
	html := `<html><body><h2>File information</h2><p>No details block here.</p></body></html>`
	records := ParseKbFilesHTML("KB0000000", html, time.Now())
	if len(records) != 0 {
		t.Errorf("attesi 0 record senza details block, ottenuti %d", len(records))
	}
}

func TestParseKbFilesHTML_EmptyHTML(t *testing.T) {
	records := ParseKbFilesHTML("KB0000000", "", time.Now())
	if len(records) != 0 {
		t.Errorf("attesi 0 record da HTML vuoto, ottenuti %d", len(records))
	}
}

func TestParseFileDate(t *testing.T) {
	tests := []struct {
		input string
		wantY int
		wantM time.Month
		wantD int
		wantNil bool
	}{
		{"22-Jan-26", 2026, time.January, 22, false},
		{"01-Mar-2025", 2025, time.March, 1, false},
		{"15-Nov-24", 2024, time.November, 15, false},
		{"", 0, 0, 0, true},
		{"invalid", 0, 0, 0, true},
		{"2026-01-22", 0, 0, 0, true}, // formato non supportato dal parser del CSV
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseFileDate(tt.input)
			if tt.wantNil {
				if got != nil {
					t.Errorf("parseFileDate(%q) = %v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseFileDate(%q) = nil, want %d-%02d-%02d", tt.input, tt.wantY, tt.wantM, tt.wantD)
			}
			if got.Year() != tt.wantY || got.Month() != tt.wantM || got.Day() != tt.wantD {
				t.Errorf("parseFileDate(%q) = %v, want %d-%02d-%02d", tt.input, got, tt.wantY, tt.wantM, tt.wantD)
			}
		})
	}
}
