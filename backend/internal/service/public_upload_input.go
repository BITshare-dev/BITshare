package service

import (
	"io"
	"path/filepath"
	"strings"

	"openshare/backend/internal/repository"
)

type normalizedUploadInput struct {
	Description string
	ReceiptCode string
	FolderID    string
	UploaderIP  string
	Files       []normalizedUploadFile
}

type normalizedUploadFile struct {
	Name         string
	RelativePath string
	RelativeDir  string
	Extension    string
	File         io.Reader
}

func (s *PublicUploadService) normalizeInput(input PublicUploadInput) (*normalizedUploadInput, error) {
	description := strings.TrimSpace(input.Description)
	if len([]rune(description)) > s.config.MaxDescriptionLength {
		return nil, ErrInvalidUploadInput
	}

	receiptCode, err := normalizeReceiptCode(input.ReceiptCode)
	if err != nil {
		return nil, ErrInvalidUploadInput
	}

	if len(input.Files) == 0 {
		return nil, ErrInvalidUploadInput
	}

	files := make([]normalizedUploadFile, 0, len(input.Files))
	for _, item := range input.Files {
		if isIgnoredUploadFile(item.Name, item.RelativePath) {
			continue
		}

		name := filepath.Base(strings.TrimSpace(item.Name))
		if name == "" || name == "." {
			return nil, ErrInvalidUploadInput
		}

		extension := strings.ToLower(strings.TrimSpace(filepath.Ext(name)))

		relativePath := repository.NormalizeRelativePathForStorage(item.RelativePath)
		if relativePath == "" {
			relativePath = name
		}
		relativeDir := ""
		if relativePath != "" {
			relativeDir = repository.NormalizeRelativePathForStorage(filepath.ToSlash(filepath.Dir(relativePath)))
		}

		files = append(files, normalizedUploadFile{
			Name:         name,
			RelativePath: relativePath,
			RelativeDir:  relativeDir,
			Extension:    extension,
			File:         item.File,
		})
	}

	return &normalizedUploadInput{
		Description: description,
		ReceiptCode: receiptCode,
		FolderID:    strings.TrimSpace(input.FolderID),
		UploaderIP:  strings.TrimSpace(input.UploaderIP),
		Files:       files,
	}, nil
}
