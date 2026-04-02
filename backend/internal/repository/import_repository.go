package repository

import (
	"errors"

	"gorm.io/gorm"
)

type ImportRepository struct {
	db *gorm.DB
}

var ErrManagedRootRequired = errors.New("managed root folder required")

func NewImportRepository(db *gorm.DB) *ImportRepository {
	return &ImportRepository{db: db}
}
