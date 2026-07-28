package db

import "strings"

func normalizeProjectName(projectName string) string {
	return strings.TrimSpace(projectName)
}
