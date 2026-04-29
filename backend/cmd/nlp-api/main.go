package main

import (
	"fmt"
	"log"

	appnlp "github.com/lgt/asr/internal/application/nlp"
	appopenplatform "github.com/lgt/asr/internal/application/openplatform"
	"github.com/lgt/asr/internal/infrastructure/nlpengine"
	"github.com/lgt/asr/internal/infrastructure/persistence"
	api "github.com/lgt/asr/internal/interfaces/api"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	cfg, err := pkgconfig.Load("configs/config.yaml")
	if err != nil {
		log.Fatal(err)
	}

	db, err := persistence.NewMySQL(cfg.Database, logger)
	if err != nil {
		log.Fatal(err)
	}
	if err := persistence.AutoMigrate(db); err != nil {
		log.Fatal(err)
	}

	entryRepo := persistence.NewEntryRepo(db)
	ruleRepo := persistence.NewRuleRepo(db)
	nlpService := appnlp.NewService(
		nlpengine.NewCorrector(entryRepo, ruleRepo),
		nlpengine.NewSummarizer(cfg.Services.SummaryModel),
	)
	openPlatformService := appopenplatform.NewService(
		persistence.NewOpenAppRepo(db),
		persistence.NewOpenSkillRepo(db),
		persistence.NewOpenCallLogRepo(db),
		nil,
		cfg.OpenAuth.PlatformSecret,
		cfg.OpenAuth.TokenExpiresIn,
	)
	openCallLogRepo := persistence.NewOpenCallLogRepo(db)

	router := api.NewRouter(logger)
	openProtected := router.Group("/openapi/v1/nlp", middleware.OpenAPIAudit(openCallLogRepo, "nlp.correct"), middleware.OpenAuthRequired(openPlatformService, "nlp.correct"))
	api.NewOpenAPINLPHandler(nlpService).Register(openProtected)
	if cfg.Legacy.Enabled {
		legacyHandler := api.NewLegacyNLPHandler(nlpService)
		legacyHandler.Register(router.Group("/api"))
		legacyHandler.Register(router.Group("/api/legacy"))
	}
	protected := router.Group("/api/nlp", middleware.AuthRequired(cfg.JWT.Secret))
	api.NewNLPHandler(nlpService).Register(protected)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.NLPAPIPort)
	logger.Info("nlp-api listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}
