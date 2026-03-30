package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/pkg/identity"
)

type ImportRepository struct {
	db *gorm.DB
}

var ErrManagedRootRequired = errors.New("managed root folder required")

type FolderTreeFolderRow struct {
	ID         string
	ParentID   *string
	Name       string
	SourcePath *string
	Status     model.ResourceStatus
}

type FolderTreeFileRow struct {
	ID            string
	FolderID      *string
	Title         string
	OriginalName  string
	Status        model.ResourceStatus
	Size          int64
	DownloadCount int64
}

type ManagedRootRow struct {
	ID         string
	SourcePath *string
}

type ManagedSubtreeFolderRow struct {
	ID          string
	ParentID    *string
	Name        string
	Description string
	SourcePath  *string
	Status      model.ResourceStatus
	CreatedAt   time.Time
}

type ManagedSubtreeFileRow struct {
	ID            string
	FolderID      *string
	SubmissionID  *string
	SourcePath    *string
	DiskPath      string
	Title         string
	Description   string
	OriginalName  string
	StoredName    string
	Extension     string
	MimeType      string
	Size          int64
	DownloadCount int64
	Status        model.ResourceStatus
	CreatedAt     time.Time
}

type ManagedFolderUpdate struct {
	ID         string
	ParentID   *string
	Name       string
	SourcePath string
}

type ManagedFileUpdate struct {
	ID           string
	FolderID     *string
	SourcePath   string
	DiskPath     string
	Title        string
	Description  string
	OriginalName string
	StoredName   string
	Extension    string
	MimeType     string
	Size         int64
}

type RescanSyncInput struct {
	RootFolderID     string
	OperatorID       string
	OperatorIP       string
	Detail           string
	Now              time.Time
	AddedFolders     []*model.Folder
	UpdatedFolders   []ManagedFolderUpdate
	DeletedFolderIDs []string
	AddedFiles       []*model.File
	UpdatedFiles     []ManagedFileUpdate
	DeletedFileIDs   []string
}

func NewImportRepository(db *gorm.DB) *ImportRepository {
	return &ImportRepository{db: db}
}

func (r *ImportRepository) FindFolderBySourcePath(ctx context.Context, sourcePath string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).Where("source_path = ?", sourcePath).Take(&folder).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find folder by source path: %w", err)
	}
	return &folder, nil
}

func (r *ImportRepository) ListManagedRoots(ctx context.Context) ([]ManagedRootRow, error) {
	var rows []ManagedRootRow
	if err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("id, source_path").
		Where("parent_id IS NULL").
		Where("source_path IS NOT NULL AND TRIM(source_path) <> ''").
		Order("source_path ASC").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list managed roots: %w", err)
	}
	return rows, nil
}

func (r *ImportRepository) FindFileBySourcePath(ctx context.Context, sourcePath string) (*model.File, error) {
	var file model.File
	err := r.db.WithContext(ctx).Where("source_path = ?", sourcePath).Take(&file).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find file by source path: %w", err)
	}
	return &file, nil
}

func (r *ImportRepository) FolderNameExists(ctx context.Context, parentID *string, name string) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.Folder{}).Where("LOWER(name) = LOWER(?)", name)
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check folder name conflict: %w", err)
	}
	return count > 0, nil
}

func (r *ImportRepository) FileNameExists(ctx context.Context, folderID *string, name string) (bool, error) {
	query := r.db.WithContext(ctx).Model(&model.File{}).Where("LOWER(original_name) = LOWER(?)", name)
	if folderID == nil {
		query = query.Where("folder_id IS NULL")
	} else {
		query = query.Where("folder_id = ?", *folderID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, fmt.Errorf("check file name conflict: %w", err)
	}
	return count > 0, nil
}

func (r *ImportRepository) CreateFolder(ctx context.Context, folder *model.Folder) error {
	return r.db.WithContext(ctx).Create(folder).Error
}

func (r *ImportRepository) CreateFile(ctx context.Context, file *model.File) error {
	return r.db.WithContext(ctx).Create(file).Error
}

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

func (r *ImportRepository) ListFolders(ctx context.Context) ([]FolderTreeFolderRow, error) {
	var rows []FolderTreeFolderRow
	err := r.db.WithContext(ctx).
		Model(&model.Folder{}).
		Select("id, parent_id, name, source_path, status").
		Order("name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list folders: %w", err)
	}
	return rows, nil
}

func (r *ImportRepository) ListFiles(ctx context.Context) ([]FolderTreeFileRow, error) {
	var rows []FolderTreeFileRow
	err := r.db.WithContext(ctx).
		Model(&model.File{}).
		Select("id, folder_id, title, original_name, status, size, download_count").
		Order("title ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list files: %w", err)
	}
	return rows, nil
}

func (r *ImportRepository) FindFolderByID(ctx context.Context, folderID string) (*model.Folder, error) {
	var folder model.Folder
	err := r.db.WithContext(ctx).Where("id = ?", folderID).Take(&folder).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find folder by id: %w", err)
	}
	return &folder, nil
}

func (r *ImportRepository) ListManagedSubtreeFolders(ctx context.Context, rootFolderID string) ([]ManagedSubtreeFolderRow, error) {
	query := `
		WITH RECURSIVE folder_tree(id, parent_id, name, description, source_path, status, created_at) AS (
			SELECT id, parent_id, name, description, source_path, status, created_at
			FROM folders
			WHERE id = ?
			UNION ALL
			SELECT folders.id, folders.parent_id, folders.name, folders.description, folders.source_path, folders.status, folders.created_at
			FROM folders
			JOIN folder_tree ON folders.parent_id = folder_tree.id
		)
		SELECT id, parent_id, name, description, source_path, status, created_at
		FROM folder_tree
	`

	var rows []ManagedSubtreeFolderRow
	if err := r.db.WithContext(ctx).Raw(query, rootFolderID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list managed subtree folders: %w", err)
	}
	return rows, nil
}

func (r *ImportRepository) ListManagedSubtreeFiles(ctx context.Context, rootFolderID string) ([]ManagedSubtreeFileRow, error) {
	query := `
		WITH RECURSIVE folder_tree(id) AS (
			SELECT id
			FROM folders
			WHERE id = ?
			UNION ALL
			SELECT folders.id
			FROM folders
			JOIN folder_tree ON folders.parent_id = folder_tree.id
		)
		SELECT
			files.id,
			files.folder_id,
			files.submission_id,
			files.source_path,
			files.disk_path,
			files.title,
			files.description,
			files.original_name,
			files.stored_name,
			files.extension,
			files.mime_type,
			files.size,
			files.download_count,
			files.status,
			files.created_at
		FROM files
		JOIN folder_tree ON files.folder_id = folder_tree.id
	`

	var rows []ManagedSubtreeFileRow
	if err := r.db.WithContext(ctx).Raw(query, rootFolderID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list managed subtree files: %w", err)
	}
	return rows, nil
}

func (r *ImportRepository) ApplyRescanSync(ctx context.Context, input RescanSyncInput) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(input.DeletedFileIDs) > 0 || len(input.DeletedFolderIDs) > 0 {
			if err := deleteManagedReportsTx(tx, input.DeletedFileIDs, input.DeletedFolderIDs); err != nil {
				return err
			}
			if len(input.DeletedFileIDs) > 0 {
				if err := tx.Where("id IN ?", input.DeletedFileIDs).Delete(&model.File{}).Error; err != nil {
					return fmt.Errorf("delete rescanned files: %w", err)
				}
			}
			if len(input.DeletedFolderIDs) > 0 {
				if err := tx.Where("id IN ?", input.DeletedFolderIDs).Delete(&model.Folder{}).Error; err != nil {
					return fmt.Errorf("delete rescanned folders: %w", err)
				}
			}
		}

		for _, update := range input.UpdatedFolders {
			if err := tx.Model(&model.Folder{}).
				Where("id = ?", update.ID).
				Updates(map[string]any{
					"parent_id":   update.ParentID,
					"name":        update.Name,
					"source_path": update.SourcePath,
					"status":      model.ResourceStatusActive,
					"deleted_at":  nil,
					"updated_at":  input.Now,
				}).Error; err != nil {
				return fmt.Errorf("update rescanned folder %s: %w", update.ID, err)
			}
		}

		for _, folder := range input.AddedFolders {
			if err := tx.Create(folder).Error; err != nil {
				return fmt.Errorf("create rescanned folder %s: %w", folder.ID, err)
			}
		}

		for _, update := range input.UpdatedFiles {
			if err := tx.Model(&model.File{}).
				Where("id = ?", update.ID).
				Updates(map[string]any{
					"folder_id":     update.FolderID,
					"source_path":   update.SourcePath,
					"disk_path":     update.DiskPath,
					"title":         update.Title,
					"description":   update.Description,
					"original_name": update.OriginalName,
					"stored_name":   update.StoredName,
					"extension":     update.Extension,
					"mime_type":     update.MimeType,
					"size":          update.Size,
					"status":        model.ResourceStatusActive,
					"deleted_at":    nil,
					"updated_at":    input.Now,
				}).Error; err != nil {
				return fmt.Errorf("update rescanned file %s: %w", update.ID, err)
			}
		}

		for _, file := range input.AddedFiles {
			if err := tx.Create(file).Error; err != nil {
				return fmt.Errorf("create rescanned file %s: %w", file.ID, err)
			}
		}

		if err := model.RebuildFolderStatsTx(tx); err != nil {
			return fmt.Errorf("rebuild folder stats after rescan: %w", err)
		}
		if err := model.RebuildDashboardStatsTx(tx); err != nil {
			return fmt.Errorf("rebuild dashboard stats after rescan: %w", err)
		}

		logID, err := identity.NewID()
		if err != nil {
			return fmt.Errorf("generate rescan operation log id: %w", err)
		}
		return createOperationLogTx(tx, logID, input.OperatorID, "managed_directory_rescanned", "folder", input.RootFolderID, input.Detail, input.OperatorIP, input.Now)
	})
}

func (r *ImportRepository) DeleteManagedRootWithLog(ctx context.Context, rootFolderID, operatorID, operatorIP, detail, logID string, now time.Time) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var folders []FolderTreeFolderRow
		if err := tx.Model(&model.Folder{}).
			Select("id, parent_id, name, source_path, status").
			Find(&folders).Error; err != nil {
			return fmt.Errorf("list folders for deletion: %w", err)
		}

		childrenByParent := make(map[string][]string)
		folderByID := make(map[string]FolderTreeFolderRow, len(folders))
		for _, folder := range folders {
			folderByID[folder.ID] = folder
			if folder.ParentID != nil {
				childrenByParent[*folder.ParentID] = append(childrenByParent[*folder.ParentID], folder.ID)
			}
		}

		root, ok := folderByID[rootFolderID]
		if !ok {
			return gorm.ErrRecordNotFound
		}

		if root.ParentID != nil {
			return ErrManagedRootRequired
		}

		folderIDs := []string{rootFolderID}
		queue := []string{rootFolderID}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			children := childrenByParent[current]
			folderIDs = append(folderIDs, children...)
			queue = append(queue, children...)
		}

		var fileIDs []string
		if err := tx.Model(&model.File{}).Where("folder_id IN ?", folderIDs).Pluck("id", &fileIDs).Error; err != nil {
			return fmt.Errorf("list files for deletion: %w", err)
		}

		type activeFileAggregate struct {
			Count int64
		}
		var activeFiles activeFileAggregate
		if err := tx.Model(&model.File{}).
			Select("COUNT(*) AS count").
			Where("folder_id IN ? AND status = ? AND deleted_at IS NULL", folderIDs, model.ResourceStatusActive).
			Scan(&activeFiles).Error; err != nil {
			return fmt.Errorf("aggregate files for deletion: %w", err)
		}

		type dayStat struct {
			Day   string
			Count int64
		}
		var createdDayStats []dayStat
		if err := tx.Model(&model.File{}).
			Select("DATE(created_at) AS day, COUNT(*) AS count").
			Where("folder_id IN ? AND status = ? AND deleted_at IS NULL", folderIDs, model.ResourceStatusActive).
			Group("DATE(created_at)").
			Scan(&createdDayStats).Error; err != nil {
			return fmt.Errorf("aggregate file day stats for deletion: %w", err)
		}

		var pendingReports int64
		reportQuery := tx.Model(&model.Report{}).Where("status = ?", model.ReportStatusPending)
		if len(fileIDs) > 0 {
			reportQuery = reportQuery.Where("(file_id IN ? OR folder_id IN ?)", fileIDs, folderIDs)
		} else {
			reportQuery = reportQuery.Where("folder_id IN ?", folderIDs)
		}
		if err := reportQuery.Count(&pendingReports).Error; err != nil {
			return fmt.Errorf("count pending reports for deletion: %w", err)
		}

		if len(fileIDs) > 0 {
			if err := tx.Where("file_id IN ?", fileIDs).Delete(&model.Report{}).Error; err != nil {
				return fmt.Errorf("delete file reports: %w", err)
			}
			if err := tx.Where("folder_id IN ?", folderIDs).Delete(&model.File{}).Error; err != nil {
				return fmt.Errorf("delete files: %w", err)
			}
		}

		if err := tx.Where("folder_id IN ?", folderIDs).Delete(&model.Report{}).Error; err != nil {
			return fmt.Errorf("delete folder reports: %w", err)
		}
		if err := tx.Where("id IN ?", folderIDs).Delete(&model.Folder{}).Error; err != nil {
			return fmt.Errorf("delete folders: %w", err)
		}

		if err := model.AdjustSystemStatsTx(tx, model.SystemStatsDelta{
			TotalFiles:     -activeFiles.Count,
			PendingReports: -pendingReports,
		}); err != nil {
			return fmt.Errorf("adjust deleted managed root system stats: %w", err)
		}
		for _, stat := range createdDayStats {
			dayTime, err := time.Parse("2006-01-02", stat.Day)
			if err != nil {
				return fmt.Errorf("parse deleted file day: %w", err)
			}
			if err := model.AdjustDailyStatsTx(tx, dayTime, model.DailyStatsDelta{NewFiles: -stat.Count}); err != nil {
				return fmt.Errorf("adjust deleted file daily stats: %w", err)
			}
		}
		return createOperationLogTx(tx, logID, operatorID, "managed_directory_deleted", "folder", rootFolderID, detail, operatorIP, now)
	})
}

func deleteManagedReportsTx(tx *gorm.DB, fileIDs []string, folderIDs []string) error {
	if tx == nil {
		return nil
	}

	switch {
	case len(fileIDs) > 0 && len(folderIDs) > 0:
		if err := tx.Where("file_id IN ?", fileIDs).Delete(&model.Report{}).Error; err != nil {
			return fmt.Errorf("delete rescanned file reports: %w", err)
		}
		if err := tx.Where("folder_id IN ?", folderIDs).Delete(&model.Report{}).Error; err != nil {
			return fmt.Errorf("delete rescanned folder reports: %w", err)
		}
	case len(fileIDs) > 0:
		if err := tx.Where("file_id IN ?", fileIDs).Delete(&model.Report{}).Error; err != nil {
			return fmt.Errorf("delete rescanned file reports: %w", err)
		}
	case len(folderIDs) > 0:
		if err := tx.Where("folder_id IN ?", folderIDs).Delete(&model.Report{}).Error; err != nil {
			return fmt.Errorf("delete rescanned folder reports: %w", err)
		}
	}

	return nil
}
