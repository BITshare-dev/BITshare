package model

import (
	"path/filepath"
	"strings"
)

func NormalizeManagedFileName(raw string) (string, string, bool) {
	name := filepath.Base(strings.TrimSpace(raw))
	switch name {
	case "", ".", "..":
		return "", "", false
	}

	extension := strings.ToLower(strings.TrimSpace(filepath.Ext(name)))
	return name, extension, true
}

func ManagedFileTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	title := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.TrimSpace(title) == "" {
		return name
	}
	return title
}

func BuildManagedFilePath(folderSourcePath *string, fileName string) string {
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" || folderSourcePath == nil || strings.TrimSpace(*folderSourcePath) == "" {
		return ""
	}
	return filepath.Join(strings.TrimSpace(*folderSourcePath), fileName)
}
