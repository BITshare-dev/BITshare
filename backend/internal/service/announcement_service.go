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

var (
	ErrAnnouncementNotFound     = errors.New("announcement not found")
	ErrAnnouncementInvalidInput = errors.New("invalid announcement input")
	ErrAnnouncementDeleteDenied = errors.New("announcement delete denied")
	ErrAnnouncementUpdateDenied = errors.New("announcement update denied")
	ErrAnnouncementPinDenied    = errors.New("announcement pin denied")
)

type AnnouncementService struct {
	repo      *repository.AnnouncementRepository
	adminRepo *repository.AdminRepository
	nowFunc   func() time.Time
}

type AnnouncementItem struct {
	ID          string                   `json:"id"`
	Title       string                   `json:"title"`
	Content     string                   `json:"content"`
	Status      model.AnnouncementStatus `json:"status"`
	IsPinned    bool                     `json:"is_pinned"`
	CreatedByID string                   `json:"created_by_id"`
	Creator     AnnouncementCreator      `json:"creator"`
	PublishedAt *time.Time               `json:"published_at,omitempty"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

type AnnouncementCreator struct {
	ID        string `json:"id"`
	Username  string `json:"username"`
	DisplayName string `json:"display_name"`
	AvatarURL string `json:"avatar_url"`
	Role      string `json:"role"`
}

type SaveAnnouncementInput struct {
	Title      string
	Content    string
	Status     model.AnnouncementStatus
	IsPinned   *bool
	OperatorID string
	OperatorIP string
}

func NewAnnouncementService(repo *repository.AnnouncementRepository, adminRepo *repository.AdminRepository) *AnnouncementService {
	return &AnnouncementService{
		repo:      repo,
		adminRepo: adminRepo,
		nowFunc:   func() time.Time { return time.Now().UTC() },
	}
}

func (s *AnnouncementService) ListPublic(ctx context.Context) ([]AnnouncementItem, error) {
	items, err := s.repo.ListPublic(ctx)
	if err != nil {
		return nil, err
	}
	return mapAnnouncements(items), nil
}

func (s *AnnouncementService) ListAdmin(ctx context.Context) ([]AnnouncementItem, error) {
	items, err := s.repo.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	return mapAnnouncements(items), nil
}

func (s *AnnouncementService) Create(ctx context.Context, input SaveAnnouncementInput) (*AnnouncementItem, error) {
	title, content, status, err := normalizeAnnouncementInput(input.Title, input.Content, input.Status)
	if err != nil {
		return nil, err
	}

	id, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate announcement id: %w", err)
	}
	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate announcement log id: %w", err)
	}
	now := s.nowFunc()
	isPinned, err := s.resolvePinnedValue(ctx, nil, input.OperatorID, input.IsPinned)
	if err != nil {
		return nil, err
	}
	item := &model.Announcement{
		ID:          id,
		Title:       title,
		Content:     content,
		Status:      status,
		IsPinned:    isPinned,
		CreatedByID: input.OperatorID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if status == model.AnnouncementStatusPublished {
		item.PublishedAt = &now
	}

	if err := s.repo.CreateWithLog(ctx, item, input.OperatorID, input.OperatorIP, logID); err != nil {
		return nil, fmt.Errorf("create announcement: %w", err)
	}

	created, err := s.repo.FindByID(ctx, item.ID)
	if err != nil {
		return nil, err
	}
	result := mapAnnouncements([]model.Announcement{*created})
	return &result[0], nil
}

func (s *AnnouncementService) Update(ctx context.Context, id string, input SaveAnnouncementInput) (*AnnouncementItem, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrAnnouncementNotFound
	}
	current, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if current == nil {
		return nil, ErrAnnouncementNotFound
	}
	if err := s.canUpdateAnnouncement(ctx, current, input.OperatorID); err != nil {
		return nil, err
	}

	title, content, status, err := normalizeAnnouncementInput(input.Title, input.Content, input.Status)
	if err != nil {
		return nil, err
	}
	isPinned, err := s.resolvePinnedValue(ctx, current, input.OperatorID, input.IsPinned)
	if err != nil {
		return nil, err
	}

	now := s.nowFunc()
	updates := map[string]any{
		"title":      title,
		"content":    content,
		"status":     status,
		"is_pinned":  isPinned,
		"updated_at": now,
	}
	switch status {
	case model.AnnouncementStatusPublished:
		if current.PublishedAt == nil {
			updates["published_at"] = &now
		}
	case model.AnnouncementStatusDraft, model.AnnouncementStatusHidden:
		updates["published_at"] = current.PublishedAt
	}

	logID, err := identity.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate announcement log id: %w", err)
	}
	if err := s.repo.UpdateWithLog(ctx, id, updates, input.OperatorID, input.OperatorIP, title, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, fmt.Errorf("update announcement: %w", err)
	}

	updated, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	result := mapAnnouncements([]model.Announcement{*updated})
	return &result[0], nil
}

func (s *AnnouncementService) resolvePinnedValue(
	ctx context.Context,
	current *model.Announcement,
	operatorID string,
	requested *bool,
) (bool, error) {
	operator, err := s.loadOperator(ctx, operatorID)
	if err != nil {
		return false, err
	}
	if operator == nil {
		return false, ErrAnnouncementUpdateDenied
	}

	currentPinned := current != nil && current.IsPinned
	if requested == nil {
		return currentPinned, nil
	}
	if operator.Role != string(model.AdminRoleSuperAdmin) {
		return false, ErrAnnouncementPinDenied
	}
	return *requested, nil
}

func (s *AnnouncementService) loadOperator(ctx context.Context, operatorID string) (*model.Admin, error) {
	if s.adminRepo == nil {
		return nil, nil
	}

	operator, err := s.adminRepo.FindByID(ctx, operatorID)
	if err != nil {
		return nil, fmt.Errorf("find operator admin: %w", err)
	}
	return operator, nil
}

func (s *AnnouncementService) canUpdateAnnouncement(ctx context.Context, current *model.Announcement, operatorID string) error {
	if s.adminRepo == nil {
		return nil
	}

	operator, err := s.loadOperator(ctx, operatorID)
	if err != nil {
		return err
	}
	if operator == nil {
		return ErrAnnouncementUpdateDenied
	}
	if operator.Role == string(model.AdminRoleSuperAdmin) {
		return nil
	}
	if current.CreatedByID != operator.ID {
		return ErrAnnouncementUpdateDenied
	}
	return nil
}

func (s *AnnouncementService) Delete(ctx context.Context, id string, operatorID string, operatorIP string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrAnnouncementNotFound
	}
	current, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return ErrAnnouncementNotFound
	}
	if err := s.canDeleteAnnouncement(ctx, current, operatorID); err != nil {
		return err
	}
	logID, err := identity.NewID()
	if err != nil {
		return fmt.Errorf("generate announcement log id: %w", err)
	}
	now := s.nowFunc()
	if err := s.repo.SoftDeleteWithLog(ctx, id, operatorID, operatorIP, current.Title, logID, now); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAnnouncementNotFound
		}
		return fmt.Errorf("delete announcement: %w", err)
	}
	return nil
}

func (s *AnnouncementService) canDeleteAnnouncement(ctx context.Context, current *model.Announcement, operatorID string) error {
	if s.adminRepo == nil {
		return nil
	}

	operator, err := s.loadOperator(ctx, operatorID)
	if err != nil {
		return err
	}
	if operator == nil {
		return ErrAnnouncementDeleteDenied
	}

	if operator.Role == string(model.AdminRoleSuperAdmin) {
		creator, err := s.adminRepo.FindByID(ctx, current.CreatedByID)
		if err != nil {
			return fmt.Errorf("find announcement creator: %w", err)
		}
		if creator != nil && creator.Role == string(model.AdminRoleSuperAdmin) && creator.ID != operator.ID {
			return ErrAnnouncementDeleteDenied
		}
		return nil
	}

	if current.CreatedByID != operator.ID {
		return ErrAnnouncementDeleteDenied
	}

	return nil
}

func normalizeAnnouncementInput(title string, content string, status model.AnnouncementStatus) (string, string, model.AnnouncementStatus, error) {
	title = strings.TrimSpace(title)
	content = strings.TrimSpace(content)
	if title == "" || content == "" {
		return "", "", "", ErrAnnouncementInvalidInput
	}
	switch status {
	case model.AnnouncementStatusDraft, model.AnnouncementStatusPublished, model.AnnouncementStatusHidden:
	default:
		return "", "", "", ErrAnnouncementInvalidInput
	}
	return title, content, status, nil
}

func mapAnnouncements(items []model.Announcement) []AnnouncementItem {
	result := make([]AnnouncementItem, 0, len(items))
	for _, item := range items {
		result = append(result, AnnouncementItem{
			ID:          item.ID,
			Title:       item.Title,
			Content:     item.Content,
			Status:      item.Status,
			IsPinned:    item.IsPinned,
			CreatedByID: item.CreatedByID,
			Creator: AnnouncementCreator{
				ID:          item.CreatedBy.ID,
				Username:    item.CreatedBy.Username,
				DisplayName: item.CreatedBy.DisplayName,
				AvatarURL:   item.CreatedBy.AvatarURL,
				Role:        item.CreatedBy.Role,
			},
			PublishedAt: item.PublishedAt,
			CreatedAt:   item.CreatedAt,
			UpdatedAt:   item.UpdatedAt,
		})
	}
	return result
}
