package main

import (
	"context"
	"fmt"
	"log"
	"time"

	appasr "github.com/lgt/asr/internal/application/asr"
	appterm "github.com/lgt/asr/internal/application/terminology"
	appuser "github.com/lgt/asr/internal/application/user"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"github.com/lgt/asr/internal/infrastructure/diarization"
	"github.com/lgt/asr/internal/infrastructure/nlpengine"
	"github.com/lgt/asr/internal/infrastructure/persistence"
	"github.com/lgt/asr/internal/infrastructure/postprocess"
	wfengine "github.com/lgt/asr/internal/infrastructure/workflow"
	api "github.com/lgt/asr/internal/interfaces/api"
	"github.com/lgt/asr/internal/interfaces/middleware"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"go.uber.org/zap"
)

type batchEngineAdapter struct {
	client *asrengine.Client
}

func (a *batchEngineAdapter) SubmitBatch(ctx context.Context, req appasr.BatchSubmitRequest) (*appasr.BatchSubmitResult, error) {
	result, err := a.client.SubmitBatch(ctx, asrengine.BatchTranscribeRequest{
		AudioURL:      req.AudioURL,
		LocalFilePath: req.LocalFilePath,
		DictID:        req.DictID,
		Progress: func(progress asrengine.BatchTranscribeProgress) {
			if req.Progress != nil {
				req.Progress(appasr.BatchSubmitProgress{
					SegmentTotal:     progress.SegmentTotal,
					SegmentCompleted: progress.SegmentCompleted,
				})
			}
		},
	})
	if err != nil {
		return nil, err
	}

	return &appasr.BatchSubmitResult{
		TaskID:     result.TaskID,
		Status:     result.Status,
		ResultText: result.ResultText,
		Duration:   result.Duration,
	}, nil
}

func (a *batchEngineAdapter) QueryBatchTask(ctx context.Context, taskID string) (*appasr.BatchTaskStatus, error) {
	result, err := a.client.QueryBatchTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	return &appasr.BatchTaskStatus{
		Status:     result.Status,
		ResultText: result.ResultText,
		Duration:   result.Duration,
	}, nil
}

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

	userRepo := persistence.NewUserRepo(db)
	workflowRepo := persistence.NewWorkflowRepo(db)
	userService := appuser.NewService(userRepo, workflowRepo)
	if err := userService.EnsureAdmin(
		context.Background(),
		cfg.Bootstrap.AdminUsername,
		cfg.Bootstrap.AdminPassword,
		cfg.Bootstrap.AdminDisplayName,
	); err != nil {
		log.Fatal(err)
	}
	logger.Info("bootstrap admin ensured", zap.String("username", cfg.Bootstrap.AdminUsername))

	termService := appterm.NewService(
		persistence.NewDictRepo(db),
		persistence.NewEntryRepo(db),
		persistence.NewRuleRepo(db),
		persistence.NewSeedStateRepo(db),
	)
	if err := termService.EnsureSeedData(context.Background()); err != nil {
		log.Fatal(err)
	}
	logger.Info("terminology seed data ensured")

	meetingRepo := persistence.NewMeetingRepo(db)
	transcriptRepo := persistence.NewTranscriptRepo(db)
	summaryRepo := persistence.NewSummaryRepo(db)
	entryRepo := persistence.NewEntryRepo(db)
	ruleRepo := persistence.NewRuleRepo(db)
	asrEngineClient := asrengine.NewClient(cfg.Services.ASR, cfg.Services.ASRStream, cfg.Services.ASRMaxAudioSizeMB)
	corrector := nlpengine.NewCorrector(entryRepo, ruleRepo)
	summarizer := nlpengine.NewSummarizer(cfg.Services.SummaryModel)
	postProcessor := postprocess.NewBatchMeetingProcessor(meetingRepo, transcriptRepo, summaryRepo, corrector, summarizer)
	asrService := appasr.NewService(persistence.NewTaskRepo(db), &batchEngineAdapter{client: asrEngineClient}, postProcessor, cfg.Services.DashboardRetryHistoryLimit, nil)
	asrService.SetStreamSessionTTL(time.Duration(cfg.Services.ASRStreamSessionRolloverSec) * time.Second)

	// Initialize workflow engine and handlers
	engine := wfengine.NewEngine(logger)
	engine.RegisterHandler(domain.NodeTermCorrection, wfengine.NewTermCorrectionHandler(corrector))
	engine.RegisterHandler(domain.NodeFillerFilter, wfengine.NewFillerFilterHandler())
	engine.RegisterHandler(domain.NodeLLMCorrection, wfengine.NewLLMCorrectionHandler())
	engine.RegisterHandler(domain.NodeCustomRegex, wfengine.NewCustomRegexHandler())
	engine.RegisterHandler(domain.NodeMeetingSummary, wfengine.NewMeetingSummaryHandler(summarizer))
	var diarizeClient *diarization.Client
	if cfg.Services.DiarizationURL != "" {
		diarizeClient = diarization.NewClient(cfg.Services.DiarizationURL)
	}
	engine.RegisterHandler(domain.NodeSpeakerDiarize, wfengine.NewSpeakerDiarizeHandler(diarizeClient))

	workflowService := appwf.NewService(
		workflowRepo,
		persistence.NewWorkflowNodeRepo(db),
		persistence.NewWorkflowExecutionRepo(db),
		persistence.NewWorkflowNodeResultRepo(db),
		engine,
	)
	if err := workflowService.EnsureSeedTemplates(context.Background()); err != nil {
		log.Fatal(err)
	}
	logger.Info("workflow seed templates ensured")

	router := api.NewRouter(logger)
	userHandler := api.NewUserHandler(userService, cfg.JWT.Secret, cfg.JWT.ExpiresIn)
	userHandler.RegisterPublic(router.Group("/api/admin/auth"))

	protected := router.Group("/api/admin", middleware.AuthRequired(cfg.JWT.Secret))
	userHandler.RegisterProtected(protected)
	api.NewTermHandler(termService).Register(protected)
	api.NewDashboardHandler(asrService, cfg.Services.ASRBatchSyncWarnThreshold, 6).Register(protected)
	api.NewWorkflowHandler(workflowService, asrService).Register(protected)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.AdminAPIPort)
	logger.Info("admin-api listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}
