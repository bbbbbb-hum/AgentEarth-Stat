package redis

import "strings"

func BuildKey(parts ...string) string {
	//clientMu.RLock()
	prefix := normalizePrefix("ae")
	//clientMu.RUnlock()

	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		cleanParts = append(cleanParts, strings.Trim(part, ":"))
	}

	if prefix == "" {
		return strings.Join(cleanParts, ":")
	}
	if len(cleanParts) == 0 {
		return prefix
	}
	return prefix + ":" + strings.Join(cleanParts, ":")
}

func normalizePrefix(prefix string) string {
	return strings.Trim(prefix, ":")
}
