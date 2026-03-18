package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/middleware"
	"openshare/backend/internal/session"
	webui "openshare/backend/web"
)

func New(db *gorm.DB, cfg config.Config, sessionManager *session.Manager) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	engine.Use(middleware.SessionLoader(sessionManager))

	handlers := buildRouteHandlers(db, cfg, sessionManager)
	registerHealthRoutes(engine, func(ctx *gin.Context) {
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
	registerPublicRoutes(api, handlers)
	registerAdminRoutes(api, handlers)
	webui.Register(engine)

	return engine
}
