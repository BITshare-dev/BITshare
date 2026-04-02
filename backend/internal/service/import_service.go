package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/repository"
	"openshare/backend/internal/storage"
)

var (
	ErrInvalidImportPath   = errors.New("invalid import path")
	ErrFolderTreeNotFound  = errors.New("folder not found")
	ErrManagedRootRequired = errors.New("managed root folder required")
)

type ManagedDirectoryConflictError struct {
	Message string
}

func (e *ManagedDirectoryConflictError) Error() string {
	if e == nil {
		return "managed directory conflict"
	}
	return e.Message
}

type ManagedDirectoryUnavailableError struct {
	Path string
}

func (e *ManagedDirectoryUnavailableError) Error() string {
	if e == nil || strings.TrimSpace(e.Path) == "" {
		return "managed directory path is unavailable"
	}
	return fmt.Sprintf("托管目录不可用：%s", e.Path)
}

type ImportService struct {
	repository *repository.ImportRepository
	storage    *storage.Service
	nowFunc    func() time.Time
}

func NewImportService(repository *repository.ImportRepository, storageService *storage.Service) *ImportService {
	return &ImportService{
		repository: repository,
		storage:    storageService,
		nowFunc:    func() time.Time { return time.Now().UTC() },
	}
}
