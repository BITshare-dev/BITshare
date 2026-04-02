package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

func (r *ImportRepository) LogOperation(ctx context.Context, adminID, action, targetType, targetID, detail, ip string, createdAt time.Time) error {
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate operation log id: %w", err)
	}

	var adminRef *string
	if strings.TrimSpace(adminID) != "" {
		adminRef = &adminID
	}

	entry := &model.OperationLog{
		ID:         logID,
		AdminID:    adminRef,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Detail:     detail,
		IP:         ip,
		CreatedAt:  createdAt,
	}
	return r.db.WithContext(ctx).Create(entry).Error
}
