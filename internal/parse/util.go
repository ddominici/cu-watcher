package parse

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var wsRe = regexp.MustCompile(`\s+`)

func clean(s string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

func pick(m map[string]string, keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(m[k]); v != "" {
			return v
		}
	}
	return ""
}

func isStandardHeader(h string) bool {
	hl := strings.ToLower(h)
	return strings.Contains(hl, "aggiornamento cumulativo") ||
		strings.Contains(hl, "cumulative update") ||
		strings.Contains(hl, "gdr") ||
		strings.Contains(hl, "sql server") ||
		strings.Contains(hl, "sqlservr.exe") ||
		strings.Contains(hl, "analysis services") ||
		strings.Contains(hl, "msmdsrv.exe") ||
		strings.Contains(hl, "knowledge base") ||
		strings.Contains(hl, "base di conoscenza") ||
		strings.Contains(hl, "data di rilascio") ||
		strings.Contains(hl, "release date")
}

// Supporta IT/EN e rimuove eventuale giorno della settimana all'inizio.
func parseLearnDate(s string) *time.Time {
	s = clean(s)
	if s == "" {
		return nil
	}
	// rimuove "martedì ", "lunedì ", ecc.
	if parts := strings.SplitN(s, " ", 2); len(parts) == 2 && strings.HasSuffix(parts[0], "ì") {
		s = parts[1]
	}

	// prova formati comuni
	layouts := []string{
		"2 January 2006",
		"January 2, 2006",
		"2 gennaio 2006",
		"02/01/2006",
		"2006-01-02",
	}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			tt := t.UTC()
			return &tt
		}
	}
	return nil
}

func itoa(i int) string { return strconv.Itoa(i) }
