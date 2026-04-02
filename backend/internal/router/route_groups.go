package router

import (
	"github.com/gin-gonic/gin"

	"openshare/backend/internal/middleware"
	"openshare/backend/internal/model"
)

func registerHealthRoutes(engine *gin.Engine, healthCheck gin.HandlerFunc) {
	engine.GET("/healthz", healthCheck)
}

func registerPublicRoutes(api *gin.RouterGroup, handlers *routeHandlers) {
	api.POST("/visits", handlers.siteVisit.Record)

	public := api.Group("/public")
	public.POST("/files/batch-download", handlers.publicDownload.DownloadBatch)
	public.POST("/resources/batch-download", handlers.publicDownload.DownloadResourceBatch)
	public.GET("/files/hot", handlers.publicCatalog.ListHotFiles)
	public.GET("/files/latest", handlers.publicCatalog.ListLatestFiles)
	public.GET("/files/:fileID", handlers.publicDownload.GetFileDetail)
	public.GET("/files/:fileID/download", handlers.publicDownload.DownloadFile)
	public.GET("/folders", handlers.publicCatalog.ListPublicFolders)
	public.GET("/folders/:folderID/files", handlers.publicCatalog.ListPublicFolderFiles)
	public.GET("/folders/:folderID", handlers.publicCatalog.GetPublicFolderDetail)
	public.GET("/folders/:folderID/download", handlers.publicDownload.DownloadFolder)
	public.GET("/announcements", handlers.announcement.ListPublic)
	public.GET("/receipt-code", handlers.publicReceipt.Ensure)
	public.GET("/search", handlers.search.Search)
	public.POST("/feedback", handlers.feedback.Create)
	public.GET("/feedback/:receiptCode", handlers.feedback.LookupByReceiptCode)
	public.POST("/submissions", handlers.publicUpload.CreateSubmission)
	public.GET("/submissions/:receiptCode", handlers.publicSubmission.LookupByReceiptCode)
}

func registerAdminRoutes(api *gin.RouterGroup, handlers *routeHandlers) {
	admin := api.Group("/admin")
	admin.POST("/session/login", handlers.adminAuth.Login)
	admin.POST("/session/logout", handlers.adminAuth.Logout)

	adminProtected := admin.Group("")
	adminProtected.Use(middleware.RequireAdminAuth())
	adminProtected.GET("/me", handlers.adminAuth.Me)
	adminProtected.GET("/dashboard/stats", handlers.adminDashboard.GetStats)
	adminProtected.POST("/session/change-password", handlers.adminAuth.ChangePassword)
	adminProtected.PATCH("/account/profile", handlers.adminAuth.UpdateProfile)
	adminProtected.GET("/operation-logs", handlers.operationLog.List)

	adminProtected.GET(
		"/announcements",
		middleware.RequireAdminPermission(model.AdminPermissionAnnouncements),
		handlers.announcement.ListAdmin,
	)
	adminProtected.POST(
		"/announcements",
		middleware.RequireAdminPermission(model.AdminPermissionAnnouncements),
		handlers.announcement.Create,
	)
	adminProtected.PUT(
		"/announcements/:announcementID",
		middleware.RequireAdminPermission(model.AdminPermissionAnnouncements),
		handlers.announcement.Update,
	)
	adminProtected.DELETE(
		"/announcements/:announcementID",
		middleware.RequireAdminPermission(model.AdminPermissionAnnouncements),
		handlers.announcement.Delete,
	)
	adminProtected.GET(
		"/submissions/pending",
		middleware.RequireAdminPermission(model.AdminPermissionSubmissionModeration),
		handlers.moderation.ListPendingSubmissions,
	)
	adminProtected.POST(
		"/submissions/:submissionID/approve",
		middleware.RequireAdminPermission(model.AdminPermissionSubmissionModeration),
		handlers.moderation.ApproveSubmission,
	)
	adminProtected.POST(
		"/submissions/:submissionID/reject",
		middleware.RequireAdminPermission(model.AdminPermissionSubmissionModeration),
		handlers.moderation.RejectSubmission,
	)
	adminProtected.GET(
		"/feedback",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.feedback.List,
	)
	adminProtected.POST(
		"/feedback/:feedbackID/approve",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.feedback.Approve,
	)
	adminProtected.POST(
		"/feedback/:feedbackID/reject",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.feedback.Reject,
	)
	adminProtected.POST(
		"/imports/local",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		handlers.imports.ImportLocalDirectory,
	)
	adminProtected.GET(
		"/imports/directories",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		handlers.imports.ListDirectories,
	)
	adminProtected.DELETE(
		"/imports/local/:folderID",
		middleware.RequireSuperAdmin(),
		handlers.imports.DeleteManagedDirectory,
	)
	adminProtected.POST(
		"/imports/local/:folderID/rescan",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		handlers.imports.RescanManagedDirectory,
	)
	adminProtected.GET("/folders/tree", handlers.imports.GetFolderTree)
	adminProtected.GET("/resources/files", handlers.resourceManagement.ListFiles)
	adminProtected.PUT(
		"/resources/folders/:folderID",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.resourceManagement.UpdateFolderDescription,
	)
	adminProtected.PUT(
		"/resources/files/:fileID",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.resourceManagement.UpdateFile,
	)
	adminProtected.DELETE(
		"/resources/files/:fileID",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.resourceManagement.DeleteFile,
	)
	adminProtected.DELETE(
		"/resources/folders/:folderID",
		middleware.RequireAdminPermission(model.AdminPermissionResourceModeration),
		handlers.resourceManagement.DeleteFolder,
	)

	adminProtected.GET("/admins", handlers.adminManagement.ListAdmins)
	adminProtected.POST(
		"/admins",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		handlers.adminManagement.CreateAdmin,
	)
	adminProtected.PUT(
		"/admins/:adminID",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		handlers.adminManagement.UpdateAdmin,
	)
	adminProtected.POST(
		"/admins/:adminID/reset-password",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		handlers.adminManagement.ResetPassword,
	)
	adminProtected.DELETE(
		"/admins/:adminID",
		middleware.RequireAdminPermission(model.AdminPermissionManageAdmins),
		handlers.adminManagement.DeleteAdmin,
	)

	superAdminOnly := adminProtected.Group("")
	superAdminOnly.Use(middleware.RequireSuperAdmin())
	superAdminOnly.GET("/system/settings", handlers.systemSetting.GetPolicy)
	superAdminOnly.PUT("/system/settings", handlers.systemSetting.SavePolicy)

	adminPermissionProbe := adminProtected.Group("/_internal")
	adminPermissionProbe.GET(
		"/review",
		middleware.RequireAdminPermission(model.AdminPermissionSubmissionModeration),
		handlers.adminAuth.PermissionProbe(model.AdminPermissionSubmissionModeration),
	)
	adminPermissionProbe.GET(
		"/system",
		middleware.RequireAdminPermission(model.AdminPermissionManageSystem),
		handlers.adminAuth.PermissionProbe(model.AdminPermissionManageSystem),
	)
}
