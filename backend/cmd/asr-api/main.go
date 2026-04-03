package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync/atomic"
	"time"

	appasr "github.com/lgt/asr/internal/application/asr"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"github.com/lgt/asr/internal/infrastructure/nlpengine"
	"github.com/lgt/asr/internal/infrastructure/persistence"
	"github.com/lgt/asr/internal/infrastructure/postprocess"
	api "github.com/lgt/asr/internal/interfaces/api"
	"github.com/lgt/asr/internal/interfaces/middleware"
	wsapi "github.com/lgt/asr/internal/interfaces/ws"
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

func startBatchSyncLoop(logger *zap.Logger, service *appasr.Service, intervalSec, batchSize, warnThreshold int) {
	if intervalSec <= 0 || batchSize <= 0 {
		logger.Info("batch task sync loop disabled", zap.Int("interval_sec", intervalSec), zap.Int("batch_size", batchSize))
		return
	}

	interval := time.Duration(intervalSec) * time.Second
	var running atomic.Bool
	runOnce := func() {
		if !running.CompareAndSwap(false, true) {
			logger.Info("batch task sync tick skipped because previous tick is still running")
			return
		}
		defer running.Store(false)

		timeout := 30 * time.Minute
		if timeout < interval {
			timeout = interval
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		summary, err := service.SyncPendingTasks(ctx, batchSize)
		if err != nil {
			logger.Warn("batch task sync tick failed", zap.Error(err))
			return
		}
		if summary.Scanned == 0 {
			return
		}

		for _, alert := range summary.Alerts {
			if warnThreshold > 0 && alert.FailCount < warnThreshold {
				continue
			}

			fields := []zap.Field{
				zap.Uint64("task_id", alert.TaskID),
				zap.String("external_task_id", alert.ExternalTaskID),
				zap.Int("fail_count", alert.FailCount),
				zap.String("last_sync_error", alert.LastSyncError),
			}
			if alert.NextSyncAt != nil {
				fields = append(fields, zap.Time("next_sync_at", *alert.NextSyncAt))
			}

			logger.Warn("batch task sync repeated failures", fields...)
		}

		if summary.Updated == 0 && summary.Failed == 0 {
			return
		}

		logger.Info(
			"batch task sync tick completed",
			zap.Int("scanned", summary.Scanned),
			zap.Int("updated", summary.Updated),
			zap.Int("failed", summary.Failed),
		)
	}

	go func() {
		logger.Info(
			"batch task sync loop started",
			zap.Int("interval_sec", intervalSec),
			zap.Int("batch_size", batchSize),
			zap.Int("warn_threshold", warnThreshold),
		)
		runOnce()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			runOnce()
		}
	}()
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

	taskRepo := persistence.NewTaskRepo(db)
	meetingRepo := persistence.NewMeetingRepo(db)
	transcriptRepo := persistence.NewTranscriptRepo(db)
	summaryRepo := persistence.NewSummaryRepo(db)
	entryRepo := persistence.NewEntryRepo(db)
	ruleRepo := persistence.NewRuleRepo(db)
	businessHub := wsapi.NewBusinessHub(logger)
	asrEngineClient := asrengine.NewClient(cfg.Services.ASR, cfg.Services.ASRStream, cfg.Services.ASRMaxAudioSizeMB)
	corrector := nlpengine.NewCorrector(entryRepo, ruleRepo)
	summarizer := nlpengine.NewSummarizer(cfg.Services.SummaryModel)
	postProcessor := postprocess.NewBatchMeetingProcessor(meetingRepo, transcriptRepo, summaryRepo, corrector, summarizer)
	asrService := appasr.NewService(taskRepo, &batchEngineAdapter{client: asrEngineClient}, postProcessor, cfg.Services.DashboardRetryHistoryLimit, businessHub)
	meetingService := appmeeting.NewService(meetingRepo, transcriptRepo, summaryRepo)
	startBatchSyncLoop(logger, asrService, cfg.Services.ASRBatchSyncIntervalSec, cfg.Services.ASRBatchSyncBatchSize, cfg.Services.ASRBatchSyncWarnThreshold)

	if err := os.MkdirAll(cfg.Upload.Dir, 0o755); err != nil {
		log.Fatal(err)
	}

	router := api.NewRouter(logger)
	router.Static("/uploads", cfg.Upload.Dir)
	protected := router.Group("/api", middleware.AuthRequired(cfg.JWT.Secret))
	api.NewASRHandler(asrService, cfg.Upload.Dir, cfg.Upload.PublicBaseURL, cfg.Upload.MaxAudioSizeMB).Register(protected.Group("/asr"))
	api.NewMeetingHandler(meetingService).Register(protected.Group("/meetings"))
	router.GET("/ws/events", middleware.AuthRequired(cfg.JWT.Secret), businessHub.Handle)
	router.GET("/ws/transcribe", middleware.AuthRequired(cfg.JWT.Secret), wsapi.NewStreamingHandler(asrEngineClient, logger, cfg.Services.ASRStreamSessionRolloverSec).Handle)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.ASRAPIPort)
	logger.Info("asr-api listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}
