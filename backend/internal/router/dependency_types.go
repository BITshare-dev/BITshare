package router

import (
	"openshare/backend/internal/handler"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
)

type routeHandlers struct {
	adminAuth          *handler.AdminAuthHandler
	adminDashboard     *handler.AdminDashboardHandler
	announcement       *handler.AnnouncementHandler
	adminManagement    *handler.AdminManagementHandler
	imports            *handler.ImportHandler
	moderation         *handler.ModerationHandler
	operationLog       *handler.OperationLogHandler
	publicCatalog      *handler.PublicCatalogHandler
	publicDownload     *handler.PublicDownloadHandler
	publicReceipt      *handler.PublicReceiptHandler
	publicSubmission   *handler.PublicSubmissionHandler
	publicUpload       *handler.PublicUploadHandler
	report             *handler.ReportHandler
	resourceManagement *handler.ResourceManagementHandler
	search             *handler.SearchHandler
	siteVisit          *handler.SiteVisitHandler
	systemSetting      *handler.SystemSettingHandler
}

type routeRepositories struct {
	admin              *repository.AdminRepository
	adminDashboard     *repository.AdminDashboardRepository
	announcement       *repository.AnnouncementRepository
	imports            *repository.ImportRepository
	moderation         *repository.ModerationRepository
	operationLog       *repository.OperationLogRepository
	publicCatalog      *repository.PublicCatalogRepository
	publicDownload     *repository.PublicDownloadRepository
	publicSubmission   *repository.PublicSubmissionRepository
	report             *repository.ReportRepository
	resourceManagement *repository.ResourceManagementRepository
	search             *repository.SearchRepository
	siteVisit          *repository.SiteVisitRepository
	systemSetting      *repository.SystemSettingRepository
	upload             *repository.UploadRepository
	receiptCode        *repository.ReceiptCodeRepository
}

type routeServices struct {
	adminAuth          *service.AdminAuthService
	adminDashboard     *service.AdminDashboardService
	announcement       *service.AnnouncementService
	adminManagement    *service.AdminManagementService
	imports            *service.ImportService
	moderation         *service.ModerationService
	operationLog       *service.OperationLogService
	publicCatalog      *service.PublicCatalogService
	publicDownload     *service.PublicDownloadService
	publicReceipt      *service.ReceiptCodeService
	publicSubmission   *service.PublicSubmissionService
	publicUpload       *service.PublicUploadService
	report             *service.ReportService
	resourceManagement *service.ResourceManagementService
	search             *service.SearchService
	siteVisit          *service.SiteVisitService
	systemSetting      *service.SystemSettingService
}
