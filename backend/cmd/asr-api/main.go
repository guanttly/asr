package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"

	appasr "github.com/lgt/asr/internal/application/asr"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	appvoicecommand "github.com/lgt/asr/internal/application/voicecommand"
	appvoiceprint "github.com/lgt/asr/internal/application/voiceprint"
	appwf "github.com/lgt/asr/internal/application/workflow"
	asrdomain "github.com/lgt/asr/internal/domain/asr"
	"github.com/lgt/asr/internal/infrastructure/asrengine"
	"github.com/lgt/asr/internal/infrastructure/diarization"
	"github.com/lgt/asr/internal/infrastructure/nlpengine"
	"github.com/lgt/asr/internal/infrastructure/persistence"
	"github.com/lgt/asr/internal/infrastructure/postprocess"
	wfengine "github.com/lgt/asr/internal/infrastructure/workflow"
	api "github.com/lgt/asr/internal/interfaces/api"
	"github.com/lgt/asr/internal/interfaces/middleware"
	wsapi "github.com/lgt/asr/internal/interfaces/ws"
	pkgconfig "github.com/lgt/asr/pkg/config"
	"go.uber.org/zap"

	wfdomain "github.com/lgt/asr/internal/domain/workflow"
)

type batchEngineAdapter struct {
	client *asrengine.Client
}

type meetingBatchEngineAdapter struct {
	client *asrengine.Client
}

// workflowExecutorAdapter wraps the workflow application service to satisfy postprocess.WorkflowExecutor.
type workflowExecutorAdapter struct {
	svc *appwf.Service
}

func (a *workflowExecutorAdapter) ExecuteForTask(ctx context.Context, task *asrdomain.TranscriptionTask, inputText string) (*appwf.ExecutionResponse, error) {
	if task == nil || task.WorkflowID == nil {
		return nil, nil
	}

	var (
		resp *appwf.ExecutionResponse
		err  error
	)
	if task.Type == asrdomain.TaskTypeRealtime {
		resp, err = a.svc.ExecuteWorkflowForRealtimeTask(ctx, *task.WorkflowID, task.ID, task.UserID, inputText, task.AudioURL, task.LocalFilePath)
	} else {
		resp, err = a.svc.ExecuteWorkflowForTask(ctx, *task.WorkflowID, task.ID, task.UserID, inputText, task.AudioURL, task.LocalFilePath)
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (a *workflowExecutorAdapter) ResumeForTaskFromFailure(ctx context.Context, task *asrdomain.TranscriptionTask) (*appwf.ExecutionResponse, error) {
	if task == nil || task.WorkflowID == nil {
		return nil, nil
	}

	var (
		resp *appwf.ExecutionResponse
		err  error
	)
	if task.Type == asrdomain.TaskTypeRealtime {
		resp, err = a.svc.ResumeLatestFailedExecutionForRealtimeTask(ctx, *task.WorkflowID, task.ID, task.UserID, task.AudioURL, task.LocalFilePath)
	} else {
		resp, err = a.svc.ResumeLatestFailedExecutionForTask(ctx, *task.WorkflowID, task.ID, task.UserID, task.AudioURL, task.LocalFilePath)
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (a *workflowExecutorAdapter) ExecuteMeetingSummaryWorkflow(ctx context.Context, workflowID, meetingID, userID uint64, inputText, audioURL, audioFilePath string) (*appwf.ExecutionResponse, error) {
	return a.svc.ExecuteWorkflow(ctx, workflowID, wfdomain.TriggerManual, fmt.Sprintf("meeting:%d", meetingID), inputText, &wfengine.ExecutionMeta{
		AudioURL:      audioURL,
		AudioFilePath: audioFilePath,
		UserID:        userID,
		MeetingID:     meetingID,
	})
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

func (a *batchEngineAdapter) StartStreamSession(ctx context.Context) (string, error) {
	return a.client.StartStreamSession(ctx)
}

func (a *batchEngineAdapter) PushStreamChunk(ctx context.Context, sessionID string, pcmData []byte) (*appasr.StreamChunkResponse, error) {
	result, err := a.client.PushStreamChunk(ctx, sessionID, pcmData)
	if err != nil {
		return nil, err
	}

	return &appasr.StreamChunkResponse{
		SessionID: result.SessionID,
		Language:  result.Language,
		Text:      result.Text,
	}, nil
}

func (a *batchEngineAdapter) FinishStreamSession(ctx context.Context, sessionID string) (*appasr.StreamChunkResponse, error) {
	result, err := a.client.FinishStreamSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	return &appasr.StreamChunkResponse{
		SessionID: result.SessionID,
		Language:  result.Language,
		Text:      result.Text,
	}, nil
}

func (a *meetingBatchEngineAdapter) SubmitBatch(ctx context.Context, req appmeeting.BatchSubmitRequest) (*appmeeting.BatchSubmitResult, error) {
	result, err := a.client.SubmitBatch(ctx, asrengine.BatchTranscribeRequest{
		AudioURL:      req.AudioURL,
		LocalFilePath: req.LocalFilePath,
	})
	if err != nil {
		return nil, err
	}

	return &appmeeting.BatchSubmitResult{
		TaskID:     result.TaskID,
		Status:     result.Status,
		ResultText: result.ResultText,
		Duration:   result.Duration,
	}, nil
}

func (a *meetingBatchEngineAdapter) QueryBatchTask(ctx context.Context, taskID string) (*appmeeting.BatchTaskStatus, error) {
	result, err := a.client.QueryBatchTask(ctx, taskID)
	if err != nil {
		return nil, err
	}

	return &appmeeting.BatchTaskStatus{
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

func startMeetingSyncLoop(logger *zap.Logger, service *appmeeting.Service, intervalSec, batchSize int) {
	if intervalSec <= 0 || batchSize <= 0 {
		logger.Info("meeting sync loop disabled", zap.Int("interval_sec", intervalSec), zap.Int("batch_size", batchSize))
		return
	}

	interval := time.Duration(intervalSec) * time.Second
	var running atomic.Bool
	runOnce := func() {
		if !running.CompareAndSwap(false, true) {
			logger.Info("meeting sync tick skipped because previous tick is still running")
			return
		}
		defer running.Store(false)

		timeout := 30 * time.Minute
		if timeout < interval {
			timeout = interval
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		summary, err := service.SyncPendingMeetings(ctx, batchSize)
		if err != nil {
			logger.Warn("meeting sync tick failed", zap.Error(err))
			return
		}
		if summary.Scanned == 0 || (summary.Updated == 0 && summary.Failed == 0) {
			return
		}

		logger.Info(
			"meeting sync tick completed",
			zap.Int("scanned", summary.Scanned),
			zap.Int("updated", summary.Updated),
			zap.Int("failed", summary.Failed),
		)
	}

	go func() {
		logger.Info(
			"meeting sync loop started",
			zap.Int("interval_sec", intervalSec),
			zap.Int("batch_size", batchSize),
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
	workflowRepo := persistence.NewWorkflowRepo(db)
	workflowNodeRepo := persistence.NewWorkflowNodeRepo(db)
	workflowExecRepo := persistence.NewWorkflowExecutionRepo(db)
	workflowResultRepo := persistence.NewWorkflowNodeResultRepo(db)
	businessHub := wsapi.NewBusinessHub(logger)
	asrEngineClient := asrengine.NewClient(cfg.Services.ASR, cfg.Services.ASRStream, cfg.Services.ASRMaxAudioSizeMB)
	corrector := nlpengine.NewCorrector(entryRepo, ruleRepo)
	summarizer := nlpengine.NewSummarizer(cfg.Services.SummaryModel)
	legacyPostProcessor := postprocess.NewBatchMeetingProcessor(meetingRepo, transcriptRepo, summaryRepo, corrector, summarizer)
	fillerDictRepo := persistence.NewFillerDictRepo(db)
	fillerEntryRepo := persistence.NewFillerEntryRepo(db)
	sensitiveDictRepo := persistence.NewSensitiveDictRepo(db)
	sensitiveEntryRepo := persistence.NewSensitiveEntryRepo(db)
	voiceCommandDictRepo := persistence.NewVoiceCommandDictRepo(db)
	voiceCommandEntryRepo := persistence.NewVoiceCommandEntryRepo(db)
	voiceCommandService := appvoicecommand.NewService(voiceCommandDictRepo, voiceCommandEntryRepo, persistence.NewSeedStateRepo(db))
	if err := voiceCommandService.EnsureSeedData(context.Background()); err != nil {
		log.Fatal(err)
	}

	// Build workflow engine
	engine := wfengine.NewEngine(logger)
	engine.RegisterHandler(wfdomain.NodeTermCorrection, wfengine.NewTermCorrectionHandler(corrector))
	engine.RegisterHandler(wfdomain.NodeFillerFilter, wfengine.NewFillerFilterHandler(fillerDictRepo, fillerEntryRepo))
	engine.RegisterHandler(wfdomain.NodeSensitiveFilter, wfengine.NewSensitiveFilterHandler(sensitiveDictRepo, sensitiveEntryRepo))
	engine.RegisterHandler(wfdomain.NodeLLMCorrection, wfengine.NewLLMCorrectionHandler())
	engine.RegisterHandler(wfdomain.NodeVoiceWake, wfengine.NewVoiceWakeHandler())
	engine.RegisterHandler(wfdomain.NodeVoiceIntent, wfengine.NewVoiceIntentHandler(voiceCommandDictRepo, voiceCommandEntryRepo))
	engine.RegisterHandler(wfdomain.NodeMeetingSummary, wfengine.NewMeetingSummaryHandler(summarizer))
	engine.RegisterHandler(wfdomain.NodeCustomRegex, wfengine.NewCustomRegexHandler())
	var diarizeClient *diarization.Client
	if cfg.Services.DiarizationURL != "" {
		diarizeClient = diarization.NewClient(cfg.Services.DiarizationURL)
	}
	speakerAnalysisURL := strings.TrimSpace(cfg.Services.SpeakerAnalysisURL)
	if speakerAnalysisURL == "" {
		speakerAnalysisURL = strings.TrimSpace(cfg.Services.DiarizationURL)
	}
	var speakerAnalysisClient *diarization.Client
	if speakerAnalysisURL != "" {
		speakerAnalysisClient = diarization.NewClient(speakerAnalysisURL)
	}
	engine.RegisterHandler(wfdomain.NodeSpeakerDiarize, wfengine.NewSpeakerDiarizeHandler(diarizeClient, speakerAnalysisClient))

	workflowService := appwf.NewService(workflowRepo, workflowNodeRepo, persistence.NewWorkflowNodeDefaultRepo(db), workflowExecRepo, workflowResultRepo, engine)
	workflowExecutor := &workflowExecutorAdapter{svc: workflowService}
	postProcessor := postprocess.NewWorkflowAwareProcessor(legacyPostProcessor, workflowExecutor, meetingRepo, transcriptRepo, summaryRepo)

	asrService := appasr.NewService(taskRepo, &batchEngineAdapter{client: asrEngineClient}, postProcessor, cfg.Services.DashboardRetryHistoryLimit, businessHub)
	asrService.SetStreamSessionTTL(time.Duration(cfg.Services.ASRStreamSessionRolloverSec) * time.Second)
	meetingService := appmeeting.NewService(meetingRepo, transcriptRepo, summaryRepo, workflowExecutor, &meetingBatchEngineAdapter{client: asrEngineClient}, businessHub)
	voiceprintService := appvoiceprint.NewService(diarization.NewClient(speakerAnalysisURL))
	startBatchSyncLoop(logger, asrService, cfg.Services.ASRBatchSyncIntervalSec, cfg.Services.ASRBatchSyncBatchSize, cfg.Services.ASRBatchSyncWarnThreshold)
	startMeetingSyncLoop(logger, meetingService, cfg.Services.ASRBatchSyncIntervalSec, cfg.Services.ASRBatchSyncBatchSize)

	if err := os.MkdirAll(cfg.Upload.Dir, 0o755); err != nil {
		log.Fatal(err)
	}

	router := api.NewRouter(logger)
	router.Static("/uploads", cfg.Upload.Dir)
	protected := router.Group("/api", middleware.AuthRequired(cfg.JWT.Secret))
	api.NewASRHandler(asrService, workflowService, cfg.Upload.Dir, cfg.Upload.PublicBaseURL, cfg.Upload.MaxAudioSizeMB).Register(protected.Group("/asr"))
	api.NewMeetingHandler(meetingService, workflowService, cfg.Upload.Dir, cfg.Upload.PublicBaseURL, cfg.Upload.MaxAudioSizeMB).Register(protected.Group("/meetings"))
	api.NewVoiceprintHandler(voiceprintService, cfg.Upload.MaxAudioSizeMB).Register(protected.Group("/meetings/voiceprints"))
	router.GET("/ws/events", middleware.AuthRequired(cfg.JWT.Secret), businessHub.Handle)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.ASRAPIPort)
	logger.Info("asr-api listening", zap.String("addr", addr))
	log.Fatal(router.Run(addr))
}
