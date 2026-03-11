package main

import (
	"log"

	"openshare/backend/internal/bootstrap"
	"openshare/backend/internal/config"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/router"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
	"openshare/backend/internal/storage"
	"openshare/backend/pkg/database"
)

func main() {
	cfg, err := config.Load("configs/config.default.json", "configs/config.local.json")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := database.NewSQLite(toDatabaseOptions(cfg.Database))
	if err != nil {
		log.Fatalf("init database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("get underlying sql.DB: %v", err)
	}
	defer sqlDB.Close()

	if err := storage.EnsureLayout(cfg.Storage); err != nil {
		log.Fatalf("init storage layout: %v", err)
	}

	if err := bootstrap.EnsureSchema(db); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	adminBootstrap := service.NewAdminBootstrapService(db, repository.NewAdminRepository(db))
	if err := adminBootstrap.EnsureDefaultSuperAdmin(); err != nil {
		log.Fatalf("init default super admin: %v", err)
	}

	sessionManager := session.NewManager(db, cfg.Session, repository.NewAdminSessionRepository())
	engine := router.New(db, sessionManager)

	log.Printf("OpenShare server listening on :%d", cfg.Server.Port)
	if err := engine.Run(cfg.Server.Address()); err != nil {
		log.Fatalf("run server: %v", err)
	}
}

func toDatabaseOptions(cfg config.DatabaseConfig) database.Options {
	pragmas := make([]database.Pragma, len(cfg.Pragmas))
	for i, p := range cfg.Pragmas {
		pragmas[i] = database.Pragma{Name: p.Name, Value: p.Value}
	}
	return database.Options{
		Path:      cfg.Path,
		LogLevel:  cfg.LogLevel,
		EnableWAL: cfg.EnableWAL,
		Pragmas:   pragmas,
	}
}
