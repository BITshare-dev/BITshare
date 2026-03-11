package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openshare/backend/internal/handler"
	"openshare/backend/internal/middleware"
	"openshare/backend/internal/model"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
)

func New(db *gorm.DB, sessionManager *session.Manager) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	engine.Use(middleware.SessionLoader(sessionManager))

	adminRepo := repository.NewAdminRepository(db)
	adminAuthService := service.NewAdminAuthService(db, adminRepo, sessionManager)
	adminAuthHandler := handler.NewAdminAuthHandler(adminAuthService, sessionManager)
	publicCatalogHandler := handler.NewPublicCatalogHandler(
		service.NewPublicCatalogService(repository.NewPublicCatalogRepository(db)),
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
	public := api.Group("/public")
	public.GET("/files", publicCatalogHandler.ListPublicFiles)

	admin := api.Group("/admin")
	admin.POST("/session/login", adminAuthHandler.Login)
	admin.POST("/session/logout", adminAuthHandler.Logout)

	adminProtected := admin.Group("")
	adminProtected.Use(middleware.RequireAdminAuth())
	adminProtected.GET("/me", adminAuthHandler.Me)

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
