package parse

import (
	"bytes"
	"encoding/csv"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

// ExtractKbFilesCSVLink finds the download.microsoft.com CSV link in a KB article.
// Microsoft embeds this link with text like "the list of files that are included in KB…".
func ExtractKbFilesCSVLink(html, pageURL string) string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return ""
	}
	var found string
	doc.Find("a[href]").EachWithBreak(func(_ int, a *goquery.Selection) bool {
		href, _ := a.Attr("href")
		if strings.HasSuffix(strings.ToLower(href), ".csv") {
			found = absURL(pageURL, href)
			return false
		}
		return true
	})
	return found
}

// ParseKbFilesCSV parses the multi-section CSV that Microsoft publishes for each KB.
// Format per section:
//
//	<Component name>,,,,,         ← section title (col[1] is empty)
//	File name,File version,...    ← header row (skipped)
//	filename.dll,1.2.3.4,...      ← file rows
func ParseKbFilesCSV(kb, csvText string, retrieved time.Time) []KbFileRecord {
	// Rimuove il BOM UTF-8 (ef bb bf) che Microsoft include nei CSV scaricati.
	csvText = strings.TrimPrefix(csvText, "\ufeff")

	r := csv.NewReader(strings.NewReader(csvText))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true

	var out []KbFileRecord
	component := ""

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(row) == 0 {
			continue
		}

		first := strings.TrimSpace(row[0])
		if first == "" {
			continue
		}

		// Column header row
		if strings.EqualFold(first, "file name") {
			continue
		}

		// Section title: col[0] has text but col[1] (file version) is empty
		if len(row) < 2 || strings.TrimSpace(row[1]) == "" {
			component = first
			continue
		}

		// File row
		rec := KbFileRecord{
			KbNumber:       kb,
			Component:      component,
			FileName:       first,
			FileVersion:    strings.TrimSpace(row[1]),
			RetrievedAtUTC: retrieved,
		}
		if len(row) > 2 {
			if n, err := strconv.ParseInt(strings.TrimSpace(row[2]), 10, 64); err == nil {
				rec.FileSizeBytes = n
			}
		}
		if len(row) > 3 {
			rec.FileDate = parseFileDate(strings.TrimSpace(row[3]))
		}
		if len(row) > 5 {
			rec.Platform = strings.TrimSpace(row[5])
		}

		out = append(out, rec)
	}
	return out
}

// ParseKbFilesHTML extracts the per-component file tables embedded directly in
// KB article pages (SQL Server 2019 and 2017 style). These pages wrap all file
// tables inside a <details> block whose <summary> reads
// "Cumulative Update package file information". Each component section is a
// <p>SQL Server …</p> immediately followed by a <table>.
func ParseKbFilesHTML(kb, html string, retrieved time.Time) []KbFileRecord {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil
	}

	var out []KbFileRecord

	doc.Find("details").Each(func(_ int, details *goquery.Selection) {
		summary := strings.ToLower(clean(details.Find("summary").Text()))
		if !strings.Contains(summary, "cumulative update package file information") {
			return
		}

		component := ""
		details.Contents().Each(func(_ int, node *goquery.Selection) {
			switch {
			case node.Is("p"):
				text := clean(node.Text())
				// Component paragraphs always start with "SQL Server"
				if strings.HasPrefix(text, "SQL Server") {
					component = text
				}
			case node.Is("table"):
				headers := extractHeaders(node)
				if len(headers) == 0 || !strings.EqualFold(headers[0], "file name") {
					return
				}
				node.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
					cells := tr.Find("td")
					if cells.Length() < 3 {
						return
					}
					vals := make([]string, cells.Length())
					cells.Each(func(i int, td *goquery.Selection) {
						vals[i] = clean(td.Text())
					})
					out = append(out, kbFileRecordFromRow(kb, component, vals, retrieved))
				})
			}
		})
	})

	return out
}

// kbFileRecordFromRow builds a KbFileRecord from an ordered cell/column slice:
// [0]=FileName [1]=FileVersion [2]=FileSize [3]=Date [4]=Time [5]=Platform
func kbFileRecordFromRow(kb, component string, vals []string, retrieved time.Time) KbFileRecord {
	rec := KbFileRecord{
		KbNumber:       kb,
		Component:      component,
		FileName:       vals[0],
		RetrievedAtUTC: retrieved,
	}
	if len(vals) > 1 {
		rec.FileVersion = vals[1]
	}
	if len(vals) > 2 {
		if n, err := strconv.ParseInt(vals[2], 10, 64); err == nil {
			rec.FileSizeBytes = n
		}
	}
	if len(vals) > 3 {
		rec.FileDate = parseFileDate(vals[3])
	}
	if len(vals) > 5 {
		rec.Platform = vals[5]
	}
	return rec
}

// parseFileDate handles both the CSV format "22-Jan-26" (2-digit year) and the
// HTML table format "21-Feb-2025" (4-digit year).
func parseFileDate(s string) *time.Time {
	for _, layout := range []string{"02-Jan-06", "02-Jan-2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
