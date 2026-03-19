package router

import (
	"context"

	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/handler"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
	"openshare/backend/internal/storage"
)

func buildRouteHandlers(db *gorm.DB, cfg config.Config, sessionManager *session.Manager) *routeHandlers {
	repos := buildRouteRepositories(db)
	services := buildRouteServices(db, cfg, repos, sessionManager)
	return buildHandlers(cfg, sessionManager, services)
}

func buildRouteRepositories(db *gorm.DB) *routeRepositories {
	return &routeRepositories{
		admin:              repository.NewAdminRepository(db),
		adminDashboard:     repository.NewAdminDashboardRepository(db),
		announcement:       repository.NewAnnouncementRepository(db),
		imports:            repository.NewImportRepository(db),
		moderation:         repository.NewModerationRepository(db),
		operationLog:       repository.NewOperationLogRepository(db),
		publicCatalog:      repository.NewPublicCatalogRepository(db),
		publicDownload:     repository.NewPublicDownloadRepository(db),
		publicSubmission:   repository.NewPublicSubmissionRepository(db),
		report:             repository.NewReportRepository(db),
		resourceManagement: repository.NewResourceManagementRepository(db),
		search:             repository.NewSearchRepository(db),
		siteVisit:          repository.NewSiteVisitRepository(db),
		systemSetting:      repository.NewSystemSettingRepository(db),
		upload:             repository.NewUploadRepository(db),
		receiptCode:        repository.NewReceiptCodeRepository(db),
	}
}

func buildRouteServices(
	db *gorm.DB,
	cfg config.Config,
	repos *routeRepositories,
	sessionManager *session.Manager,
) *routeServices {
	storageService := storage.NewService(cfg.Storage)
	receiptCodeService := service.NewReceiptCodeService(repos.receiptCode, cfg.Upload.ReceiptCodeLength)
	systemSettingService := service.NewSystemSettingService(repos.systemSetting, cfg)
	adminAuthService := service.NewAdminAuthService(db, repos.admin, sessionManager)
	searchService := service.NewSearchService(repos.search, systemSettingService)
	_ = searchService.RebuildAllIndexes(context.Background())

	return &routeServices{
		adminAuth:          adminAuthService,
		adminDashboard:     service.NewAdminDashboardService(repos.adminDashboard),
		announcement:       service.NewAnnouncementService(repos.announcement, repos.admin),
		adminManagement:    service.NewAdminManagementService(repos.admin),
		imports:            service.NewImportService(repos.imports, storageService, searchService),
		moderation:         service.NewModerationService(repos.moderation, storageService, searchService),
		operationLog:       service.NewOperationLogService(repos.operationLog),
		publicCatalog:      service.NewPublicCatalogService(repos.publicCatalog),
		publicDownload:     service.NewPublicDownloadService(repos.publicDownload, storageService),
		publicReceipt:      receiptCodeService,
		publicSubmission:   service.NewPublicSubmissionService(repos.publicSubmission),
		publicUpload:       service.NewPublicUploadService(cfg.Upload, repos.upload, receiptCodeService, storageService, systemSettingService),
		report:             service.NewReportService(repos.report, receiptCodeService, searchService, storageService),
		resourceManagement: service.NewResourceManagementServiceWithSettings(repos.resourceManagement, storageService, systemSettingService, searchService),
		search:             searchService,
		siteVisit:          service.NewSiteVisitService(repos.siteVisit),
		systemSetting:      systemSettingService,
	}
}

func buildHandlers(cfg config.Config, sessionManager *session.Manager, services *routeServices) *routeHandlers {
	return &routeHandlers{
		adminAuth:          handler.NewAdminAuthHandler(services.adminAuth, sessionManager),
		adminDashboard:     handler.NewAdminDashboardHandler(services.adminDashboard),
		announcement:       handler.NewAnnouncementHandler(services.announcement),
		adminManagement:    handler.NewAdminManagementHandler(services.adminManagement, services.adminAuth),
		imports:            handler.NewImportHandler(services.imports, services.adminAuth),
		moderation:         handler.NewModerationHandler(services.moderation),
		operationLog:       handler.NewOperationLogHandler(services.operationLog),
		publicCatalog:      handler.NewPublicCatalogHandler(services.publicCatalog),
		publicDownload:     handler.NewPublicDownloadHandler(services.publicDownload),
		publicReceipt:      handler.NewPublicReceiptHandler(services.publicReceipt),
		publicSubmission:   handler.NewPublicSubmissionHandler(services.publicSubmission),
		publicUpload:       handler.NewPublicUploadHandler(services.publicUpload, services.systemSetting, cfg.Upload.MaxBatchTotalSizeBytes+(1<<20)),
		report:             handler.NewReportHandler(services.report),
		resourceManagement: handler.NewResourceManagementHandler(services.resourceManagement, services.adminAuth),
		search:             handler.NewSearchHandler(services.search),
		siteVisit:          handler.NewSiteVisitHandler(services.siteVisit),
		systemSetting:      handler.NewSystemSettingHandler(services.systemSetting),
	}
}
