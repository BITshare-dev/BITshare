package service

import (
	"context"
)

func (s *PublicUploadService) effectivePolicy(ctx context.Context) SystemPolicy {
	if s.systemSetting == nil {
		return defaultSystemPolicy(s.config)
	}

	policy, err := s.systemSetting.GetPolicy(ctx)
	if err != nil || policy == nil {
		return defaultSystemPolicy(s.config)
	}

	return *policy
}

func (s *PublicUploadService) resolveReceiptCode(ctx context.Context, receiptCode string) (string, error) {
	return s.receiptCodes.ResolveForSession(ctx, receiptCode)
}
