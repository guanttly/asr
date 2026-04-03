package main

import (
	"fmt"
	"log"

	appnlp "github.com/lgt/asr/internal/application/nlp"
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

	router := api.NewRouter(logger)
	protected := router.Group("/api/nlp", middleware.AuthRequired(cfg.JWT.Secret))
	api.NewNLPHandler(nlpService).Register(protected)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.NLPAPIPort)
	logger.Info("nlp-api listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}
