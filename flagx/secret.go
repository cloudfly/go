package flagx

import (
	"strings"
)

// RegisterSecretFlag registers flagName as secret.
//
// This function must be called before starting logging.
// It cannot be called from concurrent goroutines.
//
// Secret flags aren't exported at `/metrics` page.
func RegisterSecretFlag(flagName string) {
	lname := strings.ToLower(flagName)
	secretFlags[lname] = true
}

var secretFlags = make(map[string]bool)

// IsSecretFlag returns true of s contains flag name with secret value, which shouldn't be exposed.
func IsSecretFlag(s string) bool {
	if strings.Contains(s, "pass") || strings.Contains(s, "key") || strings.Contains(s, "secret") || strings.Contains(s, "token") {
		return true
	}
	return secretFlags[s]
}

func toSecret(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return ""
	}
	if n <= 4 {
		return strings.Repeat("*", n)
	}
	keep := n / 4
	if keep < 1 {
		keep = 1
	}
	if keep > 4 {
		keep = 4
	}
	if keep*2 >= n {
		keep = (n - 1) / 2
	}
	return string(runes[:keep]) + strings.Repeat("*", n-keep*2) + string(runes[n-keep:])
}
