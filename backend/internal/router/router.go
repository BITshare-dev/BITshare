package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/handler"
	"openshare/backend/internal/middleware"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
	"openshare/backend/internal/storage"
)

func New(db *gorm.DB, cfg config.Config, sessionManager *session.Manager) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	engine.Use(middleware.SessionLoader(sessionManager))

	storageService := storage.NewService(cfg.Storage)
	adminRepo := repository.NewAdminRepository(db)
	systemSettingService := service.NewSystemSettingService(repository.NewSystemSettingRepository(db), cfg)
	adminAuthService := service.NewAdminAuthService(db, adminRepo, sessionManager)
	adminAuthHandler := handler.NewAdminAuthHandler(adminAuthService, sessionManager)
	adminDashboardHandler := handler.NewAdminDashboardHandler(
		service.NewAdminDashboardService(repository.NewAdminDashboardRepository(db)),
	)

	searchRepo := repository.NewSearchRepository(db)
	tagRepo := repository.NewTagRepository(db)
	searchService := service.NewSearchService(searchRepo, tagRepo, systemSettingService)
	searchHandler := handler.NewSearchHandler(searchService)
	announcementHandler := handler.NewAnnouncementHandler(
		service.NewAnnouncementService(repository.NewAnnouncementRepository(db), adminRepo),
	)
	adminManagementHandler := handler.NewAdminManagementHandler(
		service.NewAdminManagementService(adminRepo),
	)
	operationLogHandler := handler.NewOperationLogHandler(
		service.NewOperationLogService(repository.NewOperationLogRepository(db)),
	)

	importHandler := handler.NewImportHandler(
		service.NewImportService(repository.NewImportRepository(db), storageService, searchService),
	)
	tagService := service.NewTagService(tagRepo, searchService)
	tagHandler := handler.NewTagHandler(tagService)

	moderationHandler := handler.NewModerationHandler(
		service.NewModerationService(repository.NewModerationRepository(db), storageService, searchService, tagService),
	)
	resourceManagementHandler := handler.NewResourceManagementHandler(
		service.NewResourceManagementServiceWithSettings(repository.NewResourceManagementRepository(db), storageService, systemSettingService),
	)
	systemSettingHandler := handler.NewSystemSettingHandler(
		systemSettingService,
	)
	publicCatalogHandler := handler.NewPublicCatalogHandler(
		service.NewPublicCatalogService(repository.NewPublicCatalogRepository(db)),
	)
	publicDownloadHandler := handler.NewPublicDownloadHandler(
		service.NewPublicDownloadService(repository.NewPublicDownloadRepository(db), storageService),
	)
	publicSubmissionHandler := handler.NewPublicSubmissionHandler(
		service.NewPublicSubmissionService(repository.NewPublicSubmissionRepository(db)),
	)
	publicUploadHandler := handler.NewPublicUploadHandler(
		service.NewPublicUploadService(
			cfg.Upload,
			repository.NewUploadRepository(db),
			storageService,
			systemSettingService,
		),
		systemSettingService,
		cfg.Upload.MaxFileSizeBytes+(1<<20),
	)

	reportRepo := repository.NewReportRepository(db)
	reportService := service.NewReportService(reportRepo, searchService, storageService)
	reportHandler := handler.NewReportHandler(reportService)
	siteVisitHandler := handler.NewSiteVisitHandler(
		service.NewSiteVisitService(repository.NewSiteVisitRepository(db)),
	)

	engine.GET("/healthz", func(ctx *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "database handle is unavailable",
			})
			return
		}

		if err := sqlDB.Ping(); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "database ping failed",
			})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := engine.Group("/api")
	api.POST("/visits", siteVisitHandler.Record)
	public := api.Group("/public")
	public.GET("/files", publicCatalogHandler.ListPublicFiles)
	public.POST("/files/batch-download", publicDownloadHandler.DownloadBatch)
	public.GET("/files/:fileID", publicDownloadHandler.GetFileDetail)
	public.PUT("/files/:fileID", resourceManagementHandler.PublicUpdateFile)
	public.DELETE("/files/:fileID", resourceManagementHandler.PublicDeleteFile)
	public.GET("/files/:fileID/preview", publicDownloadHandler.PreviewFile)
	public.GET("/files/:fileID/download", publicDownloadHandler.DownloadFile)
	public.GET("/folders", publicCatalogHandler.ListPublicFolders)
	public.GET("/announcements", announcementHandler.ListPublic)
	public.GET("/system/policy", systemSettingHandler.GetPublicPolicy)
	public.GET("/search", searchHandler.Search)
	public.POST("/submissions", publicUploadHandler.CreateSubmission)
	public.GET("/submissions/:receiptCode", publicSubmissionHandler.LookupByReceiptCode)
	public.POST("/tag-submissions", tagHandler.SubmitCandidateTag)
	public.POST("/reports", reportHandler.CreateReport)

	admin := api.Group("/admin")
	admin.POST("/session/login", adminAuthHandler.Login)
	admin.POST("/session/logout", adminAuthHandler.Logout)

	adminProtected := admin.Group("")
	adminProtected.Use(middleware.RequireAdminAuth())
	adminProtected.GET("/me", adminAuthHandler.Me)
	adminProtected.GET("/dashboard/stats", adminDashboardHandler.GetStats)
	adminProtected.POST("/session/change-password", adminAuthHandler.ChangePassword)
	adminProtected.PATCH("/account/profile", adminAuthHandler.UpdateProfile)
	adminProtected.GET("/operation-logs", operationLogHandler.List)
	adminProtected.GET(
		"/announcements",
		middleware.RequireAdminPermission(model.AdminPermissionManageAnnouncements),
		announcementHandler.ListAdmin,
	)
	adminProtected.POST(
		"/announcements",
		middleware.RequireAdminPermission(model.AdminPermissionManageAnnouncements),
		announcementHandler.Create,
	)
	adminProtected.PUT(
		"/announcements/:announcementID",
		middleware.RequireAdminPermission(model.AdminPermissionManageAnnouncements),
		announcementHandler.Update,
	)
	adminProtected.DELETE(
		"/announcements/:announcementID",
		middleware.RequireAdminPermission(model.AdminPermissionManageAnnouncements),
		announcementHandler.Delete,
	)
	adminProtected.GET(
		"/submissions/pending",
		middleware.RequireAdminPermission(model.AdminPermissionReviewSubmissions),
		moderationHandler.ListPendingSubmissions,
	)
	adminProtected.POST(
		"/submissions/:submissionID/approve",
		middleware.RequireAdminPermission(model.AdminPermissionReviewSubmissions),
		moderationHandler.ApproveSubmission,
	)
	adminProtected.POST(
		"/submissions/:submissionID/reject",
		middleware.RequireAdminPermission(model.AdminPermissionReviewSubmissions),
		moderationHandler.RejectSubmission,
	)
	adminProtected.POST(
		"/imports/local",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		importHandler.ImportLocalDirectory,
	)
	adminProtected.GET(
		"/imports/directories",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		importHandler.ListDirectories,
	)
	adminProtected.POST(
		"/search/rebuild-index",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		searchHandler.RebuildIndex,
	)
	adminProtected.GET(
		"/folders/tree",
		importHandler.GetFolderTree,
	)
	adminProtected.GET(
		"/resources/files",
		resourceManagementHandler.ListFiles,
	)
	adminProtected.PUT(
		"/resources/files/:fileID",
		middleware.RequireAdminPermission(model.AdminPermissionEditResources),
		resourceManagementHandler.UpdateFile,
	)
	adminProtected.POST(
		"/resources/files/:fileID/offline",
		middleware.RequireAdminPermission(model.AdminPermissionDeleteResources),
		resourceManagementHandler.OfflineFile,
	)
	adminProtected.DELETE(
		"/resources/files/:fileID",
		middleware.RequireAdminPermission(model.AdminPermissionDeleteResources),
		resourceManagementHandler.DeleteFile,
	)
	adminProtected.PUT(
		"/folders/:folderID/tags",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.BindFolderTags,
	)

	// Tag management routes
	adminProtected.GET(
		"/tags",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.ListTags,
	)
	adminProtected.POST(
		"/tags",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.CreateTag,
	)
	adminProtected.PUT(
		"/tags/:tagID",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.UpdateTag,
	)
	adminProtected.DELETE(
		"/tags/:tagID",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.DeleteTag,
	)
	adminProtected.POST(
		"/tags/merge",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.MergeTags,
	)
	adminProtected.PUT(
		"/files/:fileID/tags",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.BindFileTags,
	)
	adminProtected.GET(
		"/files/:fileID/tags",
		tagHandler.GetFileTagsWithInheritance,
	)
	adminProtected.GET(
		"/folders/:folderID/tags",
		tagHandler.GetFolderTagsWithInheritance,
	)
	adminProtected.GET(
		"/tag-submissions/pending",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.ListPendingTagSubmissions,
	)
	adminProtected.POST(
		"/tag-submissions/:submissionID/approve",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.ApproveCandidateTag,
	)
	adminProtected.POST(
		"/tag-submissions/:submissionID/reject",
		middleware.RequireAdminPermission(model.AdminPermissionManageTags),
		tagHandler.RejectCandidateTag,
	)

	// Report management routes
	adminProtected.GET(
		"/reports/pending",
		middleware.RequireAdminPermission(model.AdminPermissionReviewReports),
		reportHandler.ListPendingReports,
	)
	adminProtected.POST(
		"/reports/:reportID/approve",
		middleware.RequireAdminPermission(model.AdminPermissionReviewReports),
		reportHandler.ApproveReport,
	)
	adminProtected.POST(
		"/reports/:reportID/reject",
		middleware.RequireAdminPermission(model.AdminPermissionReviewReports),
		reportHandler.RejectReport,
	)

	adminProtected.GET(
		"/admins",
		adminManagementHandler.ListAdmins,
	)
	adminProtected.POST(
		"/admins",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		adminManagementHandler.CreateAdmin,
	)
	adminProtected.PUT(
		"/admins/:adminID",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		adminManagementHandler.UpdateAdmin,
	)
	adminProtected.POST(
		"/admins/:adminID/reset-password",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		adminManagementHandler.ResetPassword,
	)
	adminProtected.DELETE(
		"/admins/:adminID",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		adminManagementHandler.DeleteAdmin,
	)
	superAdminOnly := adminProtected.Group("")
	superAdminOnly.Use(middleware.RequireSuperAdmin())
	superAdminOnly.GET("/system/settings", systemSettingHandler.GetPolicy)
	superAdminOnly.PUT("/system/settings", systemSettingHandler.SavePolicy)

	adminPermissionProbe := adminProtected.Group("/_internal")
	adminPermissionProbe.GET(
		"/review",
		middleware.RequireAdminPermission(model.AdminPermissionReviewSubmissions),
		adminAuthHandler.PermissionProbe(model.AdminPermissionReviewSubmissions),
	)
	adminPermissionProbe.GET(
		"/system",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		adminAuthHandler.PermissionProbe(model.AdminPermissionManageSystem),
	)

	return engine
}
