package main

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberlogger "github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"serpetllanos-backend/config"
	"serpetllanos-backend/pkg/audit"
	"serpetllanos-backend/pkg/response"

	// Módulo auth
	authapp  "serpetllanos-backend/internal/auth/application"
	authhttp "serpetllanos-backend/internal/auth/infrastructure/http"

	// Módulo setup
	setupapp  "serpetllanos-backend/internal/setup/application"
	setuphttp "serpetllanos-backend/internal/setup/infrastructure/http"
	setuprepo "serpetllanos-backend/internal/setup/infrastructure/repository"

	// Módulo user
	userapp  "serpetllanos-backend/internal/user/application"
	userhttp "serpetllanos-backend/internal/user/infrastructure/http"
	userrepo "serpetllanos-backend/internal/user/infrastructure/repository"
)

func main() {
	// ──────────────────────────────────────────────────────────────
	// 1. Configuración y base de datos
	// ──────────────────────────────────────────────────────────────
	cfg := config.LoadConfig()

	db := config.ConnectDB(cfg) // ejecuta migraciones y seed de permisos
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("ERROR: No se pudo obtener la instancia SQL: %v", err)
	}
	defer sqlDB.Close()

	// ──────────────────────────────────────────────────────────────
	// 2. Servicios compartidos
	// ──────────────────────────────────────────────────────────────
	auditService := audit.NewAuditService(db)

	// ──────────────────────────────────────────────────────────────
	// 3. Repositorios
	// ──────────────────────────────────────────────────────────────
	userRepo  := userrepo.NewUserRepository(db)
	setupRepo := setuprepo.NewSetupRepository(db)

	// ──────────────────────────────────────────────────────────────
	// 4. Casos de uso — Auth
	// ──────────────────────────────────────────────────────────────
	loginUC        := authapp.NewLoginUseCase(userRepo, auditService, cfg)
	refreshTokenUC := authapp.NewRefreshTokenUseCase(cfg, userRepo)
	logoutUC       := authapp.NewLogoutUseCase(auditService)

	// ──────────────────────────────────────────────────────────────
	// 5. Casos de uso — Setup
	// ──────────────────────────────────────────────────────────────
	initSystemUC := setupapp.NewInitializeSystemUseCase(setupRepo, userRepo, auditService)

	// ──────────────────────────────────────────────────────────────
	// 6. Casos de uso — Usuarios
	// ──────────────────────────────────────────────────────────────
	createUserUC     := userapp.NewCreateUserUseCase(userRepo, auditService)
	updateUserUC     := userapp.NewUpdateUserUseCase(userRepo, auditService)
	deactivateUserUC := userapp.NewDeactivateUserUseCase(userRepo, auditService)
	getUserByIdUC    := userapp.NewGetUserByIdUseCase(userRepo)
	listUsersUC      := userapp.NewListUsersUseCase(userRepo)
	changePasswordUC := userapp.NewChangePasswordUseCase(userRepo, auditService)

	// ──────────────────────────────────────────────────────────────
	// 7. Handlers HTTP
	// ──────────────────────────────────────────────────────────────
	authHandler := authhttp.NewAuthHandler(loginUC, refreshTokenUC, logoutUC)
	setupHandler := setuphttp.NewSetupHandler(initSystemUC)
	userHandler := userhttp.NewUserHandler(
		createUserUC,
		updateUserUC,
		deactivateUserUC,
		getUserByIdUC,
		listUsersUC,
		changePasswordUC,
	)

	// ──────────────────────────────────────────────────────────────
	// 8. App Fiber con error handler global
	// ──────────────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName: "Serpet Llanos API v1.0",
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return response.Error(c, fiber.StatusInternalServerError, "Error interno del servidor", "INTERNAL_ERROR")
		},
	})

	// ──────────────────────────────────────────────────────────────
	// 9. Middlewares globales
	// ──────────────────────────────────────────────────────────────
	app.Use(recover.New())
	app.Use(fiberlogger.New(fiberlogger.Config{
		Format: "[${time}] ${method} ${path} ${status} ${latency}\n",
	}))
	app.Use(cors.New())

	// ──────────────────────────────────────────────────────────────
	// 10. Rutas
	// ──────────────────────────────────────────────────────────────
	api := app.Group("/api/v1")

	// Ruta de salud (pública)
	api.Get("/health", func(c *fiber.Ctx) error {
		return response.Success(c, "Sistema operativo", fiber.Map{
			"status":    "ok",
			"timestamp": time.Now(),
			"version":   "1.0.0",
		})
	})

	// Setup inicial — público, solo funciona una vez
	setuphttp.SetupSetupRoutes(api, setupHandler)

	// Autenticación — público
	authhttp.SetupAuthRoutes(api, authHandler, cfg)

	// Usuarios — protegido por JWT
	userhttp.SetupUserRoutes(api, userHandler, cfg)

	// ──────────────────────────────────────────────────────────────
	// 11. Iniciar servidor
	// ──────────────────────────────────────────────────────────────
	log.Printf("[SERVER] Iniciando en http://localhost:%s (entorno: %s)", cfg.AppPort, cfg.AppEnv)
	log.Fatal(app.Listen(":" + cfg.AppPort))
}
