package pages

import "strconv"

// intStr converts an int to its string representation.
// Used in templ expressions where a string is required.
func intStr(n int) string {
	return strconv.Itoa(n)
}
