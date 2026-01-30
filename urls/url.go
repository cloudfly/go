package urls

import "strings"

func Join(items ...string) string {
	var s []string
	for _, item := range items {
		item = strings.TrimFunc(item, func(s rune) bool {
			return s == '/' || s == '.'
		})
		if item != "" {
			s = append(s, item)
		}
	}
	return strings.Join(s, "/")
}
