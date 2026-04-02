package model

import "time"

// ---------------------------------------------------------------------------
// Primary key type
// ---------------------------------------------------------------------------

// EntityID is a TEXT UUID primary key, decoupled from auto-increment.
type EntityID = string

// ---------------------------------------------------------------------------
// Status enumerations
// ---------------------------------------------------------------------------

// AdminStatus represents the lifecycle state of an admin account.
type AdminStatus string

const (
	AdminStatusActive   AdminStatus = "active"
	AdminStatusDisabled AdminStatus = "disabled"
)

// SubmissionStatus represents the moderation state of an upload submission.
type SubmissionStatus string

const (
	SubmissionStatusPending  SubmissionStatus = "pending"
	SubmissionStatusApproved SubmissionStatus = "approved"
	SubmissionStatusRejected SubmissionStatus = "rejected"
)

// AnnouncementStatus represents the publish state of an announcement.
type AnnouncementStatus string

const (
	AnnouncementStatusDraft     AnnouncementStatus = "draft"
	AnnouncementStatusPublished AnnouncementStatus = "published"
	AnnouncementStatusHidden    AnnouncementStatus = "hidden"
)

// FeedbackStatus represents the moderation state of a feedback record.
type FeedbackStatus string

const (
	FeedbackStatusPending  FeedbackStatus = "pending"
	FeedbackStatusApproved FeedbackStatus = "approved"
	FeedbackStatusRejected FeedbackStatus = "rejected"
)

// ---------------------------------------------------------------------------
// Core entities
// ---------------------------------------------------------------------------

// Admin represents a privileged operator in the management backend.
type Admin struct {
	ID           EntityID    `gorm:"column:id;type:text;primaryKey"`
	Username     string      `gorm:"column:username;type:text;not null;uniqueIndex:ux_admins_username"`
	DisplayName  string      `gorm:"column:display_name;type:text;not null;default:''"`
	AvatarURL    string      `gorm:"column:avatar_url;type:text;not null;default:''"`
	PasswordHash string      `gorm:"column:password_hash;type:text;not null"`
	Role         string      `gorm:"column:role;type:text;not null"` // super_admin | admin
	Permissions  string      `gorm:"column:permissions;type:text;not null;default:''"`
	Status       AdminStatus `gorm:"column:status;type:text;not null;default:'active'"`
	CreatedAt    time.Time   `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time   `gorm:"column:updated_at;autoUpdateTime"`

	Sessions []AdminSession `gorm:"foreignKey:AdminID"`
}

// Folder is the hierarchical container for files and subfolders.
type Folder struct {
	ID            EntityID  `gorm:"column:id;type:text;primaryKey"`
	ParentID      *EntityID `gorm:"column:parent_id;type:text;index:idx_folders_parent_id_status"`
	SourcePath    *string   `gorm:"column:source_path;type:text;uniqueIndex:ux_folders_source_path"`
	Name          string    `gorm:"column:name;type:text;not null"`
	Description   string    `gorm:"column:description;type:text;not null;default:''"`
	FileCount     int64     `gorm:"column:file_count;type:integer;not null;default:0"`
	TotalSize     int64     `gorm:"column:total_size;type:integer;not null;default:0"`
	DownloadCount int64     `gorm:"column:download_count;type:integer;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index:idx_folders_created_at,sort:desc"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Parent   *Folder  `gorm:"foreignKey:ParentID"`
	Children []Folder `gorm:"foreignKey:ParentID"`
	Files    []File   `gorm:"foreignKey:FolderID"`
}

// File is the managed resource metadata stored in SQLite.
type File struct {
	ID            EntityID  `gorm:"column:id;type:text;primaryKey"`
	FolderID      *EntityID `gorm:"column:folder_id;type:text;index:idx_files_folder_id"`
	Name          string    `gorm:"column:name;type:text;not null;default:''"`
	Description   string    `gorm:"column:description;type:text;not null;default:''"`
	Extension     string    `gorm:"column:extension;type:text;not null;default:''"`
	MimeType      string    `gorm:"column:mime_type;type:text;not null;default:''"`
	Size          int64     `gorm:"column:size;type:integer;not null;default:0"`
	DownloadCount int64     `gorm:"column:download_count;type:integer;not null;default:0"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime;index:idx_files_created_at,sort:desc"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Folder *Folder `gorm:"foreignKey:FolderID"`
}

// Submission tracks an upload request from staging through moderation.
type Submission struct {
	ID           EntityID         `gorm:"column:id;type:text;primaryKey"`
	ReceiptCode  string           `gorm:"column:receipt_code;type:text;not null;index:idx_submissions_receipt_code"`
	FolderID     *EntityID        `gorm:"column:folder_id;type:text;index:idx_submissions_folder_id"`
	FileID       *EntityID        `gorm:"column:file_id;type:text;index:idx_submissions_file_id"`
	Name         string           `gorm:"column:name;type:text;not null;default:''"`
	Description  string           `gorm:"column:description;type:text;not null;default:''"`
	RelativePath string           `gorm:"column:relative_path;type:text;not null;default:''"`
	Extension    string           `gorm:"column:extension;type:text;not null;default:''"`
	MimeType     string           `gorm:"column:mime_type;type:text;not null;default:''"`
	Size         int64            `gorm:"column:size;type:integer;not null;default:0"`
	StagingPath  string           `gorm:"column:staging_path;type:text;not null;default:''"`
	Status       SubmissionStatus `gorm:"column:status;type:text;not null;default:'pending';index:idx_submissions_status_created_at"`
	ReviewReason string           `gorm:"column:review_reason;type:text;not null;default:''"`
	UploaderIP   string           `gorm:"column:uploader_ip;type:text;not null;default:''"`
	ReviewerID   *EntityID        `gorm:"column:reviewer_id;type:text;index:idx_submissions_reviewer_id_reviewed_at"`
	ReviewedAt   *time.Time       `gorm:"column:reviewed_at;type:datetime;index:idx_submissions_reviewer_id_reviewed_at,sort:desc;index:idx_submissions_reviewed_at,sort:desc"`
	CreatedAt    time.Time        `gorm:"column:created_at;autoCreateTime;index:idx_submissions_status_created_at,sort:desc"`
	UpdatedAt    time.Time        `gorm:"column:updated_at;autoUpdateTime"`

	// Relations
	Reviewer *Admin  `gorm:"foreignKey:ReviewerID"`
	Folder   *Folder `gorm:"foreignKey:FolderID;constraint:OnDelete:SET NULL;"`
	File     *File   `gorm:"foreignKey:FileID;constraint:OnDelete:SET NULL;"`
}

// Feedback stores user feedback against a managed file or folder.
type Feedback struct {
	ID           EntityID       `gorm:"column:id;type:text;primaryKey"`
	ReceiptCode  string         `gorm:"column:receipt_code;type:text;not null;default:'';index:idx_feedbacks_receipt_code"`
	FileID       *EntityID      `gorm:"column:file_id;type:text;index:idx_feedbacks_file_id"`
	FolderID     *EntityID      `gorm:"column:folder_id;type:text;index:idx_feedbacks_folder_id"`
	TargetName   string         `gorm:"column:target_name;type:text;not null;default:''"`
	TargetPath   string         `gorm:"column:target_path;type:text;not null;default:''"`
	TargetType   string         `gorm:"column:target_type;type:text;not null;default:''"`
	Description  string         `gorm:"column:description;type:text;not null;default:''"`
	ReporterIP   string         `gorm:"column:reporter_ip;type:text;not null;default:''"`
	Status       FeedbackStatus `gorm:"column:status;type:text;not null;default:'pending';index:idx_feedbacks_status_created_at"`
	ReviewReason string         `gorm:"column:review_reason;type:text;not null;default:''"`
	ReviewerID   *EntityID      `gorm:"column:reviewer_id;type:text;index:idx_feedbacks_reviewer_id_reviewed_at"`
	ReviewedAt   *time.Time     `gorm:"column:reviewed_at;type:datetime;index:idx_feedbacks_reviewer_id_reviewed_at,sort:desc"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime;index:idx_feedbacks_status_created_at,sort:desc"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime"`

	File     *File   `gorm:"foreignKey:FileID;constraint:OnDelete:SET NULL;"`
	Folder   *Folder `gorm:"foreignKey:FolderID;constraint:OnDelete:SET NULL;"`
	Reviewer *Admin  `gorm:"foreignKey:ReviewerID"`
}

// Announcement is a publishable notice shown on the homepage.
type Announcement struct {
	ID          EntityID           `gorm:"column:id;type:text;primaryKey"`
	Title       string             `gorm:"column:title;type:text;not null"`
	Content     string             `gorm:"column:content;type:text;not null;default:''"`
	Status      AnnouncementStatus `gorm:"column:status;type:text;not null;default:'draft';index:idx_announcements_status_published_at"`
	IsPinned    bool               `gorm:"column:is_pinned;type:boolean;not null;default:false;index:idx_announcements_is_pinned"`
	CreatedByID EntityID           `gorm:"column:created_by_id;type:text;not null"`
	PublishedAt *time.Time         `gorm:"column:published_at;type:datetime;index:idx_announcements_status_published_at,sort:desc"`
	CreatedAt   time.Time          `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time          `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt   *time.Time         `gorm:"column:deleted_at;type:datetime"`

	CreatedBy Admin `gorm:"foreignKey:CreatedByID"`
}

// OperationLog records sensitive admin actions for auditing.
// Append-only: no updates, no soft delete.
type OperationLog struct {
	ID         EntityID  `gorm:"column:id;type:text;primaryKey"`
	AdminID    *EntityID `gorm:"column:admin_id;type:text;index:idx_operation_logs_admin_id_created_at"`
	Action     string    `gorm:"column:action;type:text;not null"`
	TargetType string    `gorm:"column:target_type;type:text;not null;default:''"`
	TargetID   string    `gorm:"column:target_id;type:text;not null;default:''"`
	Detail     string    `gorm:"column:detail;type:text;not null;default:''"`
	IP         string    `gorm:"column:ip;type:text;not null;default:''"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime;index:idx_operation_logs_admin_id_created_at,sort:desc"`

	Admin *Admin `gorm:"foreignKey:AdminID"`
}

// AdminSession is the persisted management session stored in SQLite.
type AdminSession struct {
	ID             EntityID  `gorm:"column:id;type:text;primaryKey"`
	AdminID        EntityID  `gorm:"column:admin_id;type:text;not null;index:idx_admin_sessions_admin_id_expires_at"`
	TokenHash      string    `gorm:"column:token_hash;type:text;not null;uniqueIndex:ux_admin_sessions_token_hash"`
	ExpiresAt      time.Time `gorm:"column:expires_at;type:datetime;not null;index:idx_admin_sessions_admin_id_expires_at,sort:desc;index:idx_admin_sessions_expires_at"`
	LastActivityAt time.Time `gorm:"column:last_activity_at;type:datetime;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`

	Admin Admin `gorm:"foreignKey:AdminID"`
}

// SiteVisitEvent records a page-level site visit used for visit metrics.
type SiteVisitEvent struct {
	ID        EntityID  `gorm:"column:id;type:text;primaryKey"`
	Scope     string    `gorm:"column:scope;type:text;not null;default:''"`
	Path      string    `gorm:"column:path;type:text;not null;default:''"`
	IP        string    `gorm:"column:ip;type:text;not null;default:'';index:idx_site_visit_events_ip_created_at"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_site_visit_events_created_at,sort:desc"`
}

// DownloadEvent records each successful public download for time-based metrics.
type DownloadEvent struct {
	ID        EntityID  `gorm:"column:id;type:text;primaryKey"`
	FileID    EntityID  `gorm:"column:file_id;type:text;not null;index:idx_download_events_file_id_created_at"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime;index:idx_download_events_file_id_created_at,sort:desc;index:idx_download_events_created_at,sort:desc"`
}

// SystemStat stores dashboard-wide aggregate counters.
type SystemStat struct {
	Key                string    `gorm:"column:key;type:text;primaryKey"`
	TotalVisits        int64     `gorm:"column:total_visits;type:integer;not null;default:0"`
	TotalFiles         int64     `gorm:"column:total_files;type:integer;not null;default:0"`
	TotalDownloads     int64     `gorm:"column:total_downloads;type:integer;not null;default:0"`
	PendingSubmissions int64     `gorm:"column:pending_submissions;type:integer;not null;default:0"`
	PendingFeedbacks   int64     `gorm:"column:pending_feedbacks;type:integer;not null;default:0"`
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt          time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// DailyStat stores per-day dashboard increments.
type DailyStat struct {
	Day       string    `gorm:"column:day;type:text;primaryKey"`
	NewFiles  int64     `gorm:"column:new_files;type:integer;not null;default:0"`
	Downloads int64     `gorm:"column:downloads;type:integer;not null;default:0"`
	Visits    int64     `gorm:"column:visits;type:integer;not null;default:0"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// SystemSetting stores extensible JSON-backed management policy blobs.
type SystemSetting struct {
	Key         string    `gorm:"column:key;type:text;primaryKey"`
	Value       string    `gorm:"column:value;type:text;not null;default:''"`
	UpdatedByID *EntityID `gorm:"column:updated_by_id;type:text"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime"`

	UpdatedBy *Admin `gorm:"foreignKey:UpdatedByID"`
}

// ---------------------------------------------------------------------------
// Table name overrides
// ---------------------------------------------------------------------------

func (Admin) TableName() string          { return "admins" }
func (Folder) TableName() string         { return "folders" }
func (File) TableName() string           { return "files" }
func (Submission) TableName() string     { return "submissions" }
func (Feedback) TableName() string       { return "feedbacks" }
func (Announcement) TableName() string   { return "announcements" }
func (OperationLog) TableName() string   { return "operation_logs" }
func (AdminSession) TableName() string   { return "admin_sessions" }
func (SiteVisitEvent) TableName() string { return "site_visit_events" }
func (DownloadEvent) TableName() string  { return "download_events" }
func (SystemSetting) TableName() string  { return "system_settings" }
func (SystemStat) TableName() string     { return "system_stats" }
func (DailyStat) TableName() string      { return "daily_stats" }
