package service

import (
	"strings"

	"openshare/backend/internal/model"
)

func normalizeTrimmedString(value string) string {
	return strings.TrimSpace(value)
}

func modelValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func folderSourcePath(folder *model.Folder) *string {
	if folder == nil {
		return nil
	}
	return folder.SourcePath
}
