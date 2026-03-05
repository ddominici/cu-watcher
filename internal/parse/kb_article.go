package parse

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

var versionRe = regexp.MustCompile(`(?i)\b(\d+\.\d+\.\d+\.\d+)\b`)

func ParseKbArticle(kb, url, html string, retrieved time.Time) (KbArticle, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader([]byte(html)))
	if err != nil {
		return KbArticle{}, err
	}

	title := clean(doc.Find("h1").First().Text())

	article := doc.Find("article").First()
	if article.Length() == 0 {
		article = doc.Selection
	}

	contentText := clean(article.Text())
	contentHTML, _ := article.Html()

	// headings + links
	var headings []map[string]string
	article.Find("h2,h3").Each(func(_ int, h *goquery.Selection) {
		headings = append(headings, map[string]string{
			"level": h.Get(0).Data,
			"text":  clean(h.Text()),
		})
	})

	var links []map[string]string
	article.Find("a").Each(func(_ int, a *goquery.Selection) {
		href, _ := a.Attr("href")
		if href == "" {
			return
		}
		links = append(links, map[string]string{
			"text": clean(a.Text()),
			"href": href,
		})
		if len(links) > 500 {
			return
		}
	})

	sections := map[string]any{"headings": headings, "links": links}
	sectionsJSON, _ := json.Marshal(sections)

	appliesTo := ""
	doc.Find("li").EachWithBreak(func(_ int, li *goquery.Selection) bool {
		t := clean(li.Text())
		if strings.HasPrefix(strings.ToLower(t), "si applica a") || strings.HasPrefix(strings.ToLower(t), "applies to") {
			appliesTo = t
			return false
		}
		return true
	})

	var productVer string
	if m := versionRe.FindStringSubmatch(contentText); len(m) > 1 {
		productVer = m[1]
	}

	// release date (euristica leggera; spesso è già nelle tabelle build-versions)
	var rd *time.Time
	if x := parseLearnDate(findAfterLabel(contentText, []string{"Data di rilascio:", "Release Date:"})); x != nil {
		rd = x
	}

	return KbArticle{
		KbNumber:       kb,
		URL:            url,
		Title:          title,
		AppliesTo:      appliesTo,
		ReleaseDate:    rd,
		ProductVersion: productVer,
		RetrievedAtUTC: retrieved,
		ContentText:    contentText,
		ContentHTML:    contentHTML,
		SectionsJSON:   string(sectionsJSON),
		ExtraJSON:      "",
	}, nil
}

func findAfterLabel(text string, labels []string) string {
	lt := strings.ToLower(text)
	for _, lab := range labels {
		i := strings.Index(lt, strings.ToLower(lab))
		if i >= 0 {
			rest := text[i+len(lab):]
			if len(rest) > 80 {
				rest = rest[:80]
			}
			return rest
		}
	}
	return ""
}
