package problems

import "strings"

// NormalizeTestData turns CRLF and lone CR into LF and ends non-empty data with LF. Authors may write cases on
// Windows, and Python's input() on Linux keeps the CR.
func NormalizeTestData(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}
