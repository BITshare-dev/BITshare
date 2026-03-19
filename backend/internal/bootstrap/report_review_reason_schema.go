package bootstrap

import (
	"fmt"
	"strings"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
)

func normalizeReportReviewReasons(db *gorm.DB) error {
	var reports []model.Report
	if err := db.
		Select("id, review_reason").
		Where("review_reason <> ''").
		Find(&reports).Error; err != nil {
		return fmt.Errorf("load report review reasons: %w", err)
	}

	for _, report := range reports {
		normalized := normalizeReviewReasonText(report.ReviewReason)
		if normalized == report.ReviewReason {
			continue
		}
		if err := db.Model(&model.Report{}).
			Where("id = ?", report.ID).
			Update("review_reason", normalized).Error; err != nil {
			return fmt.Errorf("normalize report review reason %s: %w", report.ID, err)
		}
	}

	return nil
}

func normalizeReviewReasonText(value string) string {
	text := strings.TrimSpace(value)
	for _, marker := range []string{"处理说明=", "驳回说明="} {
		if idx := strings.Index(text, marker); idx >= 0 {
			return strings.TrimSpace(text[idx+len(marker):])
		}
	}
	return text
}
