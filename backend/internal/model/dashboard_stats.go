package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const GlobalSystemStatsKey = "global"

func (f *File) AfterCreate(tx *gorm.DB) error {
	if err := AdjustFolderStatsTx(tx, f.FolderID, f.Size, f.DownloadCount, 1); err != nil {
		return err
	}
	if err := AdjustSystemStatsTx(tx, SystemStatsDelta{
		TotalFiles: 1,
	}); err != nil {
		return err
	}
	return AdjustDailyStatsTx(tx, f.CreatedAt, DailyStatsDelta{NewFiles: 1})
}

func (submission *Submission) AfterCreate(tx *gorm.DB) error {
	if submission.Status != SubmissionStatusPending {
		return nil
	}
	return AdjustSystemStatsTx(tx, SystemStatsDelta{PendingSubmissions: 1})
}

func (feedback *Feedback) AfterCreate(tx *gorm.DB) error {
	if feedback.Status != FeedbackStatusPending {
		return nil
	}
	return AdjustSystemStatsTx(tx, SystemStatsDelta{PendingFeedbacks: 1})
}

func (event *DownloadEvent) AfterCreate(tx *gorm.DB) error {
	if err := AdjustSystemStatsTx(tx, SystemStatsDelta{TotalDownloads: 1}); err != nil {
		return err
	}
	return AdjustDailyStatsTx(tx, event.CreatedAt, DailyStatsDelta{Downloads: 1})
}

func (event *SiteVisitEvent) AfterCreate(tx *gorm.DB) error {
	return RecordSiteVisitStatsTx(tx, event.CreatedAt)
}

type SystemStatsDelta struct {
	TotalVisits        int64
	TotalFiles         int64
	TotalDownloads     int64
	PendingSubmissions int64
	PendingFeedbacks   int64
}

type DailyStatsDelta struct {
	NewFiles  int64
	Downloads int64
	Visits    int64
}

func AdjustSystemStatsTx(tx *gorm.DB, delta SystemStatsDelta) error {
	if tx == nil {
		return nil
	}
	if delta == (SystemStatsDelta{}) {
		return nil
	}

	now := time.Now().UTC()
	row := SystemStat{
		Key:                GlobalSystemStatsKey,
		TotalVisits:        delta.TotalVisits,
		TotalFiles:         delta.TotalFiles,
		TotalDownloads:     delta.TotalDownloads,
		PendingSubmissions: delta.PendingSubmissions,
		PendingFeedbacks:   delta.PendingFeedbacks,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "key"}},
		DoUpdates: clause.Assignments(map[string]any{
			"total_visits":        gorm.Expr("total_visits + ?", delta.TotalVisits),
			"total_files":         gorm.Expr("total_files + ?", delta.TotalFiles),
			"total_downloads":     gorm.Expr("total_downloads + ?", delta.TotalDownloads),
			"pending_submissions": gorm.Expr("pending_submissions + ?", delta.PendingSubmissions),
			"pending_feedbacks":   gorm.Expr("pending_feedbacks + ?", delta.PendingFeedbacks),
			"updated_at":          now,
		}),
	}).Create(&row).Error
}

func AdjustDailyStatsTx(tx *gorm.DB, at time.Time, delta DailyStatsDelta) error {
	if tx == nil {
		return nil
	}
	if delta == (DailyStatsDelta{}) {
		return nil
	}

	day := statsDay(at)
	now := time.Now().UTC()
	row := DailyStat{
		Day:       day,
		NewFiles:  delta.NewFiles,
		Downloads: delta.Downloads,
		Visits:    delta.Visits,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "day"}},
		DoUpdates: clause.Assignments(map[string]any{
			"new_files":  gorm.Expr("new_files + ?", delta.NewFiles),
			"downloads":  gorm.Expr("downloads + ?", delta.Downloads),
			"visits":     gorm.Expr("visits + ?", delta.Visits),
			"updated_at": now,
		}),
	}).Create(&row).Error
}

func RecordSiteVisitStatsTx(tx *gorm.DB, createdAt time.Time) error {
	if tx == nil {
		return nil
	}

	if err := AdjustSystemStatsTx(tx, SystemStatsDelta{TotalVisits: 1}); err != nil {
		return err
	}
	return AdjustDailyStatsTx(tx, createdAt, DailyStatsDelta{Visits: 1})
}

func statsDay(at time.Time) string {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return at.UTC().Format("2006-01-02")
}

func EnsureSystemStatsRowTx(tx *gorm.DB) error {
	if tx == nil {
		return nil
	}

	now := time.Now().UTC()
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&SystemStat{
		Key:       GlobalSystemStatsKey,
		CreatedAt: now,
		UpdatedAt: now,
	}).Error
}
