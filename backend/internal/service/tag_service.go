package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/pkg/identity"
)

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

var (
	ErrTagNotFound             = errors.New("tag not found")
	ErrTagNameEmpty            = errors.New("tag name is empty")
	ErrTagNameTooLong          = errors.New("tag name exceeds maximum length")
	ErrTagNameDuplicate        = errors.New("tag with this name already exists")
	ErrTagMergeSameTag         = errors.New("source and target tag are the same")
	ErrTagMergeTargetNotFound  = errors.New("merge target tag not found")
	ErrTagSubmissionNotFound   = errors.New("tag submission not found")
	ErrTagSubmissionNotPending = errors.New("tag submission is not pending")
	ErrFileNotFound            = errors.New("file not found")
	ErrTagBindInvalidInput     = errors.New("invalid tag binding input")
	ErrTagSubmissionNameEmpty  = errors.New("proposed tag name is empty")
)

const maxTagNameLength = 32

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

type TagService struct {
	repo    *repository.TagRepository
	search  *SearchService
	nowFunc func() time.Time
}

func NewTagService(repo *repository.TagRepository, searchService *SearchService) *TagService {
	return &TagService{
		repo:    repo,
		search:  searchService,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
}

// ---------------------------------------------------------------------------
// 6.1 Tag CRUD
// ---------------------------------------------------------------------------

type TagItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	FileCount   int64     `json:"file_count"`
	FolderCount int64     `json:"folder_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateTagInput struct {
	Name       string
	AdminID    string
	OperatorIP string
}

type UpdateTagInput struct {
	TagID      string
	Name       string
	AdminID    string
	OperatorIP string
}

func (s *TagService) CreateTag(ctx context.Context, input CreateTagInput) (*TagItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrTagNameEmpty
	}
	if len([]rune(name)) > maxTagNameLength {
		return nil, ErrTagNameTooLong
	}

	normalized := strings.ToLower(name)
	existing, err := s.repo.FindTagByNormalizedName(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("check tag name duplicate: %w", err)
	}
	if existing != nil {
		return nil, ErrTagNameDuplicate
	}

	tagID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate tag id: %w", err)
	}
	now := s.nowFunc()
	tag := &model.Tag{
		ID:             tagID,
		Name:           name,
		NameNormalized: normalized,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateTag(ctx, tag); err != nil {
		return nil, fmt.Errorf("create tag: %w", err)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "tag_created", "tag", tagID, name, input.OperatorIP, now)

	return &TagItem{
		ID:        tagID,
		Name:      name,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (s *TagService) UpdateTag(ctx context.Context, input UpdateTagInput) (*TagItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrTagNameEmpty
	}
	if len([]rune(name)) > maxTagNameLength {
		return nil, ErrTagNameTooLong
	}

	tag, err := s.repo.FindTagByID(ctx, strings.TrimSpace(input.TagID))
	if err != nil {
		return nil, fmt.Errorf("find tag: %w", err)
	}
	if tag == nil {
		return nil, ErrTagNotFound
	}

	normalized := strings.ToLower(name)
	if normalized != tag.NameNormalized {
		existing, err := s.repo.FindTagByNormalizedName(ctx, normalized)
		if err != nil {
			return nil, fmt.Errorf("check tag name duplicate: %w", err)
		}
		if existing != nil {
			return nil, ErrTagNameDuplicate
		}
	}

	now := s.nowFunc()
	if err := s.repo.UpdateTag(ctx, tag.ID, map[string]any{
		"name":            name,
		"name_normalized": normalized,
		"updated_at":      now,
	}); err != nil {
		return nil, fmt.Errorf("update tag: %w", err)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "tag_updated", "tag", tag.ID,
		fmt.Sprintf("%s -> %s", tag.Name, name), input.OperatorIP, now)

	return &TagItem{
		ID:        tag.ID,
		Name:      name,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: now,
	}, nil
}

func (s *TagService) DeleteTag(ctx context.Context, tagID, adminID, operatorIP string) error {
	tag, err := s.repo.FindTagByID(ctx, strings.TrimSpace(tagID))
	if err != nil {
		return fmt.Errorf("find tag: %w", err)
	}
	if tag == nil {
		return ErrTagNotFound
	}

	if err := s.repo.DeleteTagAssociations(ctx, tag.ID); err != nil {
		return fmt.Errorf("delete tag associations: %w", err)
	}

	now := s.nowFunc()
	if err := s.repo.SoftDeleteTag(ctx, tag.ID, now); err != nil {
		return fmt.Errorf("soft delete tag: %w", err)
	}

	_ = s.repo.LogOperation(ctx, adminID, "tag_deleted", "tag", tag.ID, tag.Name, operatorIP, now)
	return nil
}

func (s *TagService) ListTags(ctx context.Context) ([]TagItem, error) {
	rows, err := s.repo.ListTags(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	items := make([]TagItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, TagItem{
			ID:          row.ID,
			Name:        row.Name,
			FileCount:   row.FileCount,
			FolderCount: row.FolderCount,
			CreatedAt:   row.CreatedAt,
			UpdatedAt:   row.UpdatedAt,
		})
	}
	return items, nil
}

// ---------------------------------------------------------------------------
// 6.2 File / Folder binding
// ---------------------------------------------------------------------------

type BindFileTagsInput struct {
	FileID     string
	TagNames   []string
	AdminID    string
	OperatorIP string
}

type BindFolderTagsInput struct {
	FolderID   string
	TagNames   []string
	AdminID    string
	OperatorIP string
}

func (s *TagService) BindFileTags(ctx context.Context, input BindFileTagsInput) error {
	file, err := s.repo.FindFileByID(ctx, strings.TrimSpace(input.FileID))
	if err != nil {
		return fmt.Errorf("find file: %w", err)
	}
	if file == nil {
		return ErrFileNotFound
	}

	tagIDs, err := s.resolveTagIDs(ctx, input.TagNames, 20, maxTagNameLength)
	if err != nil {
		return err
	}

	if err := s.repo.ReplaceFileTags(ctx, file.ID, tagIDs); err != nil {
		return fmt.Errorf("replace file tags: %w", err)
	}

	// Re-index file after tag change.
	if s.search != nil {
		_ = s.search.IndexFile(ctx, file.ID, file.Title)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "file_tags_updated", "file", file.ID,
		strings.Join(input.TagNames, ","), input.OperatorIP, s.nowFunc())
	return nil
}

func (s *TagService) BindFolderTags(ctx context.Context, input BindFolderTagsInput) error {
	folder, err := s.repo.FindFolderByID(ctx, strings.TrimSpace(input.FolderID))
	if err != nil {
		return fmt.Errorf("find folder: %w", err)
	}
	if folder == nil {
		return ErrFolderTreeNotFound
	}

	tagIDs, err := s.resolveTagIDs(ctx, input.TagNames, 20, maxTagNameLength)
	if err != nil {
		return err
	}

	if err := s.repo.ReplaceFolderTags(ctx, folder.ID, tagIDs); err != nil {
		return fmt.Errorf("replace folder tags: %w", err)
	}

	// Re-index folder after tag change.
	if s.search != nil {
		_ = s.search.IndexFolder(ctx, folder.ID, folder.Name)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "folder_tags_updated", "folder", folder.ID,
		strings.Join(input.TagNames, ","), input.OperatorIP, s.nowFunc())
	return nil
}

// resolveTagIDs normalizes tag names, deduplicates, and finds-or-creates each tag.
func (s *TagService) resolveTagIDs(ctx context.Context, tagNames []string, maxCount, maxLength int) ([]string, error) {
	names, err := normalizeTags(tagNames, maxCount, maxLength)
	if err != nil {
		return nil, ErrTagBindInvalidInput
	}

	if len(names) == 0 {
		return nil, nil
	}

	normalized := make([]string, len(names))
	for i, n := range names {
		normalized[i] = strings.ToLower(n)
	}

	existing, err := s.repo.FindTagsByNormalizedNames(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("find tags by names: %w", err)
	}
	byNormalized := make(map[string]model.Tag, len(existing))
	for _, t := range existing {
		byNormalized[t.NameNormalized] = t
	}

	now := s.nowFunc()
	tagIDs := make([]string, 0, len(names))
	for i, name := range names {
		key := normalized[i]
		tag, ok := byNormalized[key]
		if !ok {
			tagID, err := identity.NewID()
			if err != nil {
				return nil, fmt.Errorf("generate tag id: %w", err)
			}
			tag = model.Tag{
				ID:             tagID,
				Name:           name,
				NameNormalized: key,
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := s.repo.CreateTag(ctx, &tag); err != nil {
				return nil, fmt.Errorf("create tag: %w", err)
			}
			byNormalized[key] = tag
		}
		tagIDs = append(tagIDs, tag.ID)
	}
	return tagIDs, nil
}

// ---------------------------------------------------------------------------
// 6.3 Tag inheritance
// ---------------------------------------------------------------------------

type FileTagDetail struct {
	FileID        string   `json:"file_id"`
	DirectTags    []string `json:"direct_tags"`
	InheritedTags []string `json:"inherited_tags"`
}

// GetFileTagsWithInheritance returns direct and inherited tags for a file.
func (s *TagService) GetFileTagsWithInheritance(ctx context.Context, fileID string) (*FileTagDetail, error) {
	file, err := s.repo.FindFileByID(ctx, strings.TrimSpace(fileID))
	if err != nil {
		return nil, fmt.Errorf("find file: %w", err)
	}
	if file == nil {
		return nil, ErrFileNotFound
	}

	directRows, err := s.repo.ListFileTagRows(ctx, []string{file.ID})
	if err != nil {
		return nil, fmt.Errorf("list file tags: %w", err)
	}
	directTags := make([]string, 0, len(directRows))
	directSet := make(map[string]struct{}, len(directRows))
	for _, row := range directRows {
		directTags = append(directTags, row.TagName)
		directSet[strings.ToLower(row.TagName)] = struct{}{}
	}

	inheritedTags := make([]string, 0)
	if file.FolderID != nil {
		inherited, err := s.collectInheritedTags(ctx, *file.FolderID, directSet)
		if err != nil {
			return nil, err
		}
		inheritedTags = inherited
	}

	return &FileTagDetail{
		FileID:        file.ID,
		DirectTags:    directTags,
		InheritedTags: inheritedTags,
	}, nil
}

type FolderTagDetail struct {
	FolderID      string   `json:"folder_id"`
	DirectTags    []string `json:"direct_tags"`
	InheritedTags []string `json:"inherited_tags"`
}

// GetFolderTagsWithInheritance returns direct and inherited tags for a folder.
func (s *TagService) GetFolderTagsWithInheritance(ctx context.Context, folderID string) (*FolderTagDetail, error) {
	folder, err := s.repo.FindFolderByID(ctx, strings.TrimSpace(folderID))
	if err != nil {
		return nil, fmt.Errorf("find folder: %w", err)
	}
	if folder == nil {
		return nil, ErrFolderTreeNotFound
	}

	directRows, err := s.repo.ListFolderTagRows(ctx, []string{folder.ID})
	if err != nil {
		return nil, fmt.Errorf("list folder tags: %w", err)
	}
	directTags := make([]string, 0, len(directRows))
	directSet := make(map[string]struct{}, len(directRows))
	for _, row := range directRows {
		directTags = append(directTags, row.TagName)
		directSet[strings.ToLower(row.TagName)] = struct{}{}
	}

	inheritedTags := make([]string, 0)
	if folder.ParentID != nil {
		inherited, err := s.collectInheritedTags(ctx, *folder.ParentID, directSet)
		if err != nil {
			return nil, err
		}
		inheritedTags = inherited
	}

	return &FolderTagDetail{
		FolderID:      folder.ID,
		DirectTags:    directTags,
		InheritedTags: inheritedTags,
	}, nil
}

// collectInheritedTags walks up the folder tree from startFolderID, collecting
// all tags from ancestor folders that aren't already in excludeSet.
func (s *TagService) collectInheritedTags(ctx context.Context, startFolderID string, excludeSet map[string]struct{}) ([]string, error) {
	ancestorIDs, err := s.repo.GetFolderAncestorIDs(ctx, startFolderID)
	if err != nil {
		return nil, fmt.Errorf("get folder ancestors: %w", err)
	}
	// Include startFolderID itself (the immediate parent when called from file context).
	allFolderIDs := append([]string{startFolderID}, ancestorIDs...)

	if len(allFolderIDs) == 0 {
		return nil, nil
	}

	folderTagRows, err := s.repo.ListFolderTagRows(ctx, allFolderIDs)
	if err != nil {
		return nil, fmt.Errorf("list ancestor folder tags: %w", err)
	}

	seen := make(map[string]struct{}, len(excludeSet))
	for k, v := range excludeSet {
		seen[k] = v
	}
	inherited := make([]string, 0)
	for _, row := range folderTagRows {
		key := strings.ToLower(row.TagName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		inherited = append(inherited, row.TagName)
	}
	return inherited, nil
}

// ---------------------------------------------------------------------------
// 6.4 User candidate tag submissions
// ---------------------------------------------------------------------------

type SubmitCandidateTagInput struct {
	ProposedName string
	SubmitterIP  string
}

type CandidateTagResult struct {
	ID           string                    `json:"id"`
	ProposedName string                    `json:"proposed_name"`
	Status       model.TagSubmissionStatus `json:"status"`
	CreatedAt    time.Time                 `json:"created_at"`
}

func (s *TagService) SubmitCandidateTag(ctx context.Context, input SubmitCandidateTagInput) (*CandidateTagResult, error) {
	name := strings.TrimSpace(input.ProposedName)
	if name == "" {
		return nil, ErrTagSubmissionNameEmpty
	}
	if len([]rune(name)) > maxTagNameLength {
		return nil, ErrTagNameTooLong
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate tag submission id: %w", err)
	}
	now := s.nowFunc()
	submission := &model.TagSubmission{
		ID:           id,
		ProposedName: name,
		Status:       model.TagSubmissionStatusPending,
		SubmitterIP:  strings.TrimSpace(input.SubmitterIP),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.repo.CreateTagSubmission(ctx, submission); err != nil {
		return nil, fmt.Errorf("create tag submission: %w", err)
	}
	return &CandidateTagResult{
		ID:           id,
		ProposedName: name,
		Status:       model.TagSubmissionStatusPending,
		CreatedAt:    now,
	}, nil
}

type PendingTagSubmissionItem struct {
	ID           string    `json:"id"`
	ProposedName string    `json:"proposed_name"`
	SubmitterIP  string    `json:"submitter_ip"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *TagService) ListPendingTagSubmissions(ctx context.Context) ([]PendingTagSubmissionItem, error) {
	rows, err := s.repo.ListPendingTagSubmissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list pending tag submissions: %w", err)
	}
	items := make([]PendingTagSubmissionItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, PendingTagSubmissionItem{
			ID:           row.ID,
			ProposedName: row.ProposedName,
			SubmitterIP:  row.SubmitterIP,
			CreatedAt:    row.CreatedAt,
		})
	}
	return items, nil
}

type ApproveCandidateTagInput struct {
	SubmissionID string
	FinalName    string // If non-empty, overrides the proposed name.
	AdminID      string
	OperatorIP   string
}

func (s *TagService) ApproveCandidateTag(ctx context.Context, input ApproveCandidateTagInput) (*TagItem, error) {
	submission, err := s.repo.FindTagSubmissionByID(ctx, strings.TrimSpace(input.SubmissionID))
	if err != nil {
		return nil, fmt.Errorf("find tag submission: %w", err)
	}
	if submission == nil {
		return nil, ErrTagSubmissionNotFound
	}
	if submission.Status != model.TagSubmissionStatusPending {
		return nil, ErrTagSubmissionNotPending
	}

	// Determine the final tag name: use override if provided, else the proposed name.
	tagName := strings.TrimSpace(input.FinalName)
	if tagName == "" {
		tagName = submission.ProposedName
	}
	if len([]rune(tagName)) > maxTagNameLength {
		return nil, ErrTagNameTooLong
	}

	normalized := strings.ToLower(tagName)
	now := s.nowFunc()

	// Find-or-create the formal tag.
	existing, err := s.repo.FindTagByNormalizedName(ctx, normalized)
	if err != nil {
		return nil, fmt.Errorf("check existing tag: %w", err)
	}
	var tag *model.Tag
	if existing != nil {
		tag = existing
	} else {
		tagID, err := identity.NewID()
		if err != nil {
			return nil, fmt.Errorf("generate tag id: %w", err)
		}
		tag = &model.Tag{
			ID:             tagID,
			Name:           tagName,
			NameNormalized: normalized,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := s.repo.CreateTag(ctx, tag); err != nil {
			return nil, fmt.Errorf("create formal tag: %w", err)
		}
	}

	if err := s.repo.ApproveTagSubmission(ctx, submission.ID, tag.ID, input.AdminID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTagSubmissionNotPending
		}
		return nil, fmt.Errorf("approve tag submission: %w", err)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "tag_submission_approved", "tag_submission", submission.ID,
		tagName, input.OperatorIP, now)

	return &TagItem{
		ID:        tag.ID,
		Name:      tag.Name,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	}, nil
}

type RejectCandidateTagInput struct {
	SubmissionID string
	RejectReason string
	AdminID      string
	OperatorIP   string
}

func (s *TagService) RejectCandidateTag(ctx context.Context, input RejectCandidateTagInput) error {
	reason := strings.TrimSpace(input.RejectReason)
	if reason == "" {
		return ErrRejectReasonRequired
	}

	submission, err := s.repo.FindTagSubmissionByID(ctx, strings.TrimSpace(input.SubmissionID))
	if err != nil {
		return fmt.Errorf("find tag submission: %w", err)
	}
	if submission == nil {
		return ErrTagSubmissionNotFound
	}
	if submission.Status != model.TagSubmissionStatusPending {
		return ErrTagSubmissionNotPending
	}

	now := s.nowFunc()
	if err := s.repo.RejectTagSubmission(ctx, submission.ID, input.AdminID, reason, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrTagSubmissionNotPending
		}
		return fmt.Errorf("reject tag submission: %w", err)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "tag_submission_rejected", "tag_submission", submission.ID,
		reason, input.OperatorIP, now)
	return nil
}

// ---------------------------------------------------------------------------
// 6.5 Tag governance — merge
// ---------------------------------------------------------------------------

type MergeTagsInput struct {
	SourceTagID string
	TargetTagID string
	AdminID     string
	OperatorIP  string
}

func (s *TagService) MergeTags(ctx context.Context, input MergeTagsInput) error {
	sourceID := strings.TrimSpace(input.SourceTagID)
	targetID := strings.TrimSpace(input.TargetTagID)
	if sourceID == targetID {
		return ErrTagMergeSameTag
	}

	source, err := s.repo.FindTagByID(ctx, sourceID)
	if err != nil {
		return fmt.Errorf("find source tag: %w", err)
	}
	if source == nil {
		return ErrTagNotFound
	}

	target, err := s.repo.FindTagByID(ctx, targetID)
	if err != nil {
		return fmt.Errorf("find target tag: %w", err)
	}
	if target == nil {
		return ErrTagMergeTargetNotFound
	}

	now := s.nowFunc()
	if err := s.repo.MergeTags(ctx, sourceID, targetID, now); err != nil {
		return fmt.Errorf("merge tags: %w", err)
	}

	_ = s.repo.LogOperation(ctx, input.AdminID, "tag_merged", "tag", sourceID,
		fmt.Sprintf("%s -> %s", source.Name, target.Name), input.OperatorIP, now)
	return nil
}
