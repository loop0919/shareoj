package problems

import "strings"

// NormalizeNewlines turns CRLF and lone CR into LF. Authors may write cases on Windows, and Python's input() on Linux
// keeps the CR.
func NormalizeNewlines(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
