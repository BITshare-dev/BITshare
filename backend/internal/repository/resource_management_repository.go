package repository

import (
	"gorm.io/gorm"
)

type ResourceManagementRepository struct {
	db *gorm.DB
}

func NewResourceManagementRepository(db *gorm.DB) *ResourceManagementRepository {
	return &ResourceManagementRepository{db: db}
}
