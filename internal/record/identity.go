package record

import "strings"

// ValidLogIdentity is the shared note/sync entry-header validation.
func ValidLogIdentity(author, machine string) bool {
	return author != "" && machine != "" && !strings.ContainsAny(author, "@ \t\r\n") && !strings.ContainsAny(machine, " \t\r\n")
}

// MergeIdentity uses a safe tool identity when machine configuration cannot
// produce a valid log entry. Notes instead require an explicit valid identity.
func MergeIdentity(author, machine string) (string, string) {
	if author == "" || strings.ContainsAny(author, "@ \t\r\n") {
		author = "tasktree"
	}
	if machine == "" || strings.ContainsAny(machine, " \t\r\n") {
		machine = "localhost"
	}
	return author, machine
}
