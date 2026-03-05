package parse

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var kbRe = regexp.MustCompile(`(?i)\bKB\d{6,8}\b`)

func ParseBuildVersions(major int, sourceURL, html string, retrieved time.Time) ([]BuildRow, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return nil, err
	}

	var out []BuildRow

	// pattern: h2 -> next table
	doc.Find("h2").Each(func(_ int, h2 *goquery.Selection) {
		topic := clean(h2.Text())
		if topic == "" {
			return
		}

		// find next table in siblings
		table := nextTable(h2)
		if table == nil {
			return
		}

		headers := extractHeaders(table)

		table.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
			cells := tr.Find("td")
			if cells.Length() == 0 {
				return
			}

			m := map[string]string{}
			cells.Each(func(i int, td *goquery.Selection) {
				if i < len(headers) {
					m[headers[i]] = clean(td.Text())
				}
			})

			updateName := pick(m,
				"Nome dell'aggiornamento cumulativo", "Cumulative update name",
				"Nome GDR", "GDR name",
			)
			sqlBuild := pick(m,
				"Versione build SQL Server", "SQL Server build version",
				"Versione prodotto SQL Server", "SQL Server product version",
			)
			sqlFile := pick(m,
				"Versione file SQL Server (sqlservr.exe)", "SQL Server (sqlservr.exe) file version",
				"Versione del file di SQL Server (sqlservr.exe)",
			)
			asBuild := pick(m,
				"Versione build di Analysis Services", "Analysis Services build version",
				"Versione prodotto Analysis Services",
			)
			asFile := pick(m,
				"Versione del file di Analysis Services (msmdsrv.exe)", "Analysis Services (msmdsrv.exe) file version",
			)
			kb := pick(m,
				"Numero Knowledge Base", "Knowledge Base number",
				"Numero base di conoscenza", "Numero della Knowledge Base",
			)
			dateStr := pick(m, "Data di rilascio", "Release date")

			// KB a volte è nel testo del link
			if kb == "" {
				tr.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
					t := clean(a.Text())
					if kbRe.MatchString(t) {
						kb = strings.ToUpper(kbRe.FindString(t))
						return false
					}
					return true
				})
			} else if kbRe.MatchString(kb) {
				kb = strings.ToUpper(kbRe.FindString(kb))
			}

			kbURL := ""
			tr.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
				t := clean(a.Text())
				href, _ := a.Attr("href")
				if kbRe.MatchString(t) || strings.Contains(strings.ToLower(href), "kb") {
					kbURL = absURL(sourceURL, href)
					return false
				}
				return true
			})

			rd := parseLearnDate(dateStr)

			extra := map[string]string{}
			for k, v := range m {
				if !isStandardHeader(k) {
					extra[k] = v
				}
			}

			out = append(out, BuildRow{
				MajorVersion:   major,
				Topic:          topic,
				UpdateName:     updateName,
				SqlBuild:       sqlBuild,
				SqlFileVer:     sqlFile,
				AsBuild:        asBuild,
				AsFileVer:      asFile,
				KbNumber:       kb,
				KbURL:          kbURL,
				ReleaseDate:    rd,
				SourceURL:      sourceURL,
				RetrievedAtUTC: retrieved,
				Extra:          extra,
			})
		})
	})

	return out, nil
}

func ExtractKbLinks(html, pageURL string) map[string]string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	doc.Find("a").Each(func(_ int, a *goquery.Selection) {
		t := clean(a.Text())
		if !kbRe.MatchString(t) {
			return
		}
		kb := strings.ToUpper(kbRe.FindString(t))
		href, ok := a.Attr("href")
		if !ok || href == "" {
			return
		}
		if _, exists := out[kb]; !exists {
			out[kb] = absURL(pageURL, href)
		}
	})
	return out
}

func GroupByTopicTable(rows []BuildRow) map[string][]BuildRow {
	out := map[string][]BuildRow{}
	for _, r := range rows {
		table := topicToTableName(r.MajorVersion, r.Topic)
		out[table] = append(out[table], r)
	}
	return out
}

func topicToTableName(major int, topic string) string {
	t := strings.ToLower(topic)
	slug := "Other_Builds"
	if strings.Contains(t, "aggiornamento cumulativo") || strings.Contains(t, "cumulative update") {
		slug = "CU_Builds"
	} else if strings.Contains(t, "gdr") {
		slug = "GDR_Builds"
	} else if strings.Contains(t, "azure connect") {
		slug = "AzureConnectPack_Builds"
	}
	return "Sql" + itoa(major) + "_" + slug
}

func nextTable(h2 *goquery.Selection) *goquery.Selection {
	s := h2.Parent()
	// fallback: prova siblings del nodo h2 stesso
	cur := h2
	for i := 0; i < 50; i++ {
		n := cur.Next()
		if n.Length() == 0 {
			break
		}
		if n.Is("table") {
			return n
		}
		if n.Find("table").Length() > 0 {
			return n.Find("table").First()
		}
		cur = n
	}
	_ = s
	return nil
}

func extractHeaders(table *goquery.Selection) []string {
	var headers []string
	table.Find("thead th").Each(func(_ int, th *goquery.Selection) {
		headers = append(headers, clean(th.Text()))
	})
	if len(headers) == 0 {
		// fallback first row
		table.Find("tr").First().Find("th,td").Each(func(_ int, c *goquery.Selection) {
			headers = append(headers, clean(c.Text()))
		})
	}
	return headers
}

func absURL(base, href string) string {
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err == nil && u.IsAbs() {
		return href
	}
	b, err := url.Parse(base)
	if err != nil {
		return href
	}
	return b.ResolveReference(u).String()
}
