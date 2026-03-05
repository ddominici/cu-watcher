package db

import "regexp"

var identRe = regexp.MustCompile(`[^\w]`)

func SanitizeIdentifier(s string) string {
	s = identRe.ReplaceAllString(s, "_")
	if len(s) > 128 {
		s = s[:128]
	}
	return s
}
