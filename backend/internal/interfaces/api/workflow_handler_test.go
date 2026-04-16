package api

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	appwf "github.com/lgt/asr/internal/application/workflow"
	domain "github.com/lgt/asr/internal/domain/workflow"
	"github.com/lgt/asr/internal/interfaces/middleware"
)

type workflowRepoHandlerStub struct {
	created           *domain.Workflow
	items             []*domain.Workflow
	filteredItems     []*domain.Workflow
	listFilteredCalls int
}

func (r *workflowRepoHandlerStub) Create(_ context.Context, wf *domain.Workflow) error {
	wf.ID = 301
	r.created = wf
	return nil
}

func (r *workflowRepoHandlerStub) GetByID(_ context.Context, _ uint64) (*domain.Workflow, error) {
	panic("unexpected call to GetByID")
}

func (r *workflowRepoHandlerStub) Update(_ context.Context, _ *domain.Workflow) error {
	panic("unexpected call to Update")
}

func (r *workflowRepoHandlerStub) Delete(_ context.Context, _ uint64) error {
	panic("unexpected call to Delete")
}

func (r *workflowRepoHandlerStub) List(_ context.Context, _ *domain.OwnerType, _ *uint64, _ bool, _, _ int) ([]*domain.Workflow, int64, error) {
	return r.items, int64(len(r.items)), nil
}

func (r *workflowRepoHandlerStub) ListFiltered(_ context.Context, _ *domain.OwnerType, _ *uint64, _ bool, _ domain.WorkflowListFilter, offset, limit int) ([]*domain.Workflow, int64, error) {
	r.listFilteredCalls++
	items := append([]*domain.Workflow(nil), r.filteredItems...)
	total := int64(len(items))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = len(items)
	}
	if offset >= len(items) {
		return []*domain.Workflow{}, total, nil
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	return items[offset:end], total, nil
}

type workflowNodeRepoHandlerStub struct {
	items map[uint64][]domain.Node
}

func (r *workflowNodeRepoHandlerStub) ListByWorkflow(_ context.Context, workflowID uint64) ([]domain.Node, error) {
	return r.items[workflowID], nil
}

func (r *workflowNodeRepoHandlerStub) BatchSave(_ context.Context, _ uint64, _ []domain.Node) error {
	panic("unexpected call to BatchSave")
}

func (r *workflowNodeRepoHandlerStub) DeleteByWorkflow(_ context.Context, _ uint64) error {
	panic("unexpected call to DeleteByWorkflow")
}

func TestCreateWorkflowBindsSourceIDForUserRequest(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &workflowRepoHandlerStub{}
	service := appwf.NewService(repo, nil, nil, nil, nil, nil)
	handler := NewWorkflowHandler(service, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "user"})
		c.Next()
	})
	router.POST("/workflows", handler.CreateWorkflow)

	sourceID := uint64(42)
	body, err := json.Marshal(map[string]any{
		"name":        "派生工作流",
		"description": "从模板分叉",
		"source_id":   sourceID,
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.created == nil {
		t.Fatal("expected repository Create to receive workflow")
	}
	if repo.created.SourceID == nil || *repo.created.SourceID != sourceID {
		t.Fatalf("expected repository source_id=%d, got %+v", sourceID, repo.created.SourceID)
	}
	if repo.created.OwnerType != domain.OwnerUser {
		t.Fatalf("expected owner_type=%s, got %s", domain.OwnerUser, repo.created.OwnerType)
	}
	if repo.created.OwnerID != 7 {
		t.Fatalf("expected owner_id=7, got %d", repo.created.OwnerID)
	}

	var envelope struct {
		Code    int                    `json:"code"`
		Message string                 `json:"message"`
		Data    appwf.WorkflowResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Code != 0 {
		t.Fatalf("expected success code 0, got %d", envelope.Code)
	}
	if envelope.Data.SourceID == nil || *envelope.Data.SourceID != sourceID {
		t.Fatalf("expected response source_id=%d, got %+v", sourceID, envelope.Data.SourceID)
	}
	if envelope.Data.OwnerType != domain.OwnerUser {
		t.Fatalf("expected response owner_type=%s, got %s", domain.OwnerUser, envelope.Data.OwnerType)
	}
	if envelope.Data.OwnerID != 7 {
		t.Fatalf("expected response owner_id=7, got %d", envelope.Data.OwnerID)
	}
}

func TestCreateWorkflowAsAdminDefaultsToUserWorkflow(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &workflowRepoHandlerStub{}
	service := appwf.NewService(repo, nil, nil, nil, nil, nil)
	handler := NewWorkflowHandler(service, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 99, Role: "admin"})
		c.Next()
	})
	router.POST("/workflows", handler.CreateWorkflow)

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewBufferString(`{"name":"系统模板"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.created == nil {
		t.Fatal("expected repository Create to receive workflow")
	}
	if repo.created.OwnerType != domain.OwnerUser {
		t.Fatalf("expected owner_type=%s, got %s", domain.OwnerUser, repo.created.OwnerType)
	}
	if repo.created.OwnerID != 99 {
		t.Fatalf("expected owner_id=99, got %d", repo.created.OwnerID)
	}

	var envelope struct {
		Code int                    `json:"code"`
		Data appwf.WorkflowResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if envelope.Data.OwnerType != domain.OwnerUser {
		t.Fatalf("expected response owner_type=%s, got %s", domain.OwnerUser, envelope.Data.OwnerType)
	}
	if envelope.Data.OwnerID != 99 {
		t.Fatalf("expected response owner_id=99, got %d", envelope.Data.OwnerID)
	}
}

func TestCreateWorkflowAsAdminCanExplicitlyCreateSystemWorkflow(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &workflowRepoHandlerStub{}
	service := appwf.NewService(repo, nil, nil, nil, nil, nil)
	handler := NewWorkflowHandler(service, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 99, Role: "admin"})
		c.Next()
	})
	router.POST("/workflows", handler.CreateWorkflow)

	req := httptest.NewRequest(http.MethodPost, "/workflows", bytes.NewBufferString(`{"name":"系统模板","owner_type":"system"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}
	if repo.created == nil {
		t.Fatal("expected repository Create to receive workflow")
	}
	if repo.created.OwnerType != domain.OwnerSystem {
		t.Fatalf("expected owner_type=%s, got %s", domain.OwnerSystem, repo.created.OwnerType)
	}
	if repo.created.OwnerID != 99 {
		t.Fatalf("expected owner_id=99, got %d", repo.created.OwnerID)
	}
}

func TestListWorkflowsAppliesFilterBeforePagination(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := &workflowRepoHandlerStub{
		filteredItems: []*domain.Workflow{
			{ID: 2, Name: "batch-1", WorkflowType: domain.WorkflowTypeBatch, SourceKind: domain.SourceKindBatchASR, TargetKind: domain.TargetKindTranscript},
			{ID: 3, Name: "batch-2", WorkflowType: domain.WorkflowTypeBatch, SourceKind: domain.SourceKindBatchASR, TargetKind: domain.TargetKindTranscript},
		},
	}
	nodes := &workflowNodeRepoHandlerStub{items: map[uint64][]domain.Node{
		2: {{WorkflowID: 2, NodeType: domain.NodeBatchASR, Position: 1, Enabled: true}},
		3: {{WorkflowID: 3, NodeType: domain.NodeBatchASR, Position: 1, Enabled: true}},
	}}
	service := appwf.NewService(repo, nodes, nil, nil, nil, nil)
	handler := NewWorkflowHandler(service, nil)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("auth_claims", &middleware.Claims{UserID: 7, Role: "admin"})
		c.Next()
	})
	router.GET("/workflows", handler.ListWorkflows)

	req := httptest.NewRequest(http.MethodGet, "/workflows?workflow_type=batch_transcription&include_legacy=false&offset=0&limit=1", nil)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d, body=%s", recorder.Code, recorder.Body.String())
	}

	var envelope struct {
		Code int                        `json:"code"`
		Data appwf.WorkflowListResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if repo.listFilteredCalls != 1 {
		t.Fatalf("expected repository ListFiltered to be called once, got %d", repo.listFilteredCalls)
	}
	if envelope.Data.Total != 2 {
		t.Fatalf("expected filtered total=2, got %d", envelope.Data.Total)
	}
	if len(envelope.Data.Items) != 1 {
		t.Fatalf("expected paged items=1, got %d", len(envelope.Data.Items))
	}
	if envelope.Data.Items[0].ID != 2 {
		t.Fatalf("expected first filtered workflow id=2, got %d", envelope.Data.Items[0].ID)
	}
}

func TestSaveOptionalWorkflowAudioStoresMultipartFile(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("node_type", "speaker_diarize"); err != nil {
		t.Fatalf("write node_type: %v", err)
	}
	if err := writer.WriteField("config", `{}`); err != nil {
		t.Fatalf("write config: %v", err)
	}
	part, err := writer.CreateFormFile("file", "sample.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write([]byte("RIFF0000WAVEfmt ")); err != nil {
		t.Fatalf("write sample file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/workflows/test-node", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request = req

	path, cleanup, err := saveOptionalWorkflowAudio(ctx, "workflow-test-node")
	if err != nil {
		t.Fatalf("saveOptionalWorkflowAudio returned error: %v", err)
	}
	defer cleanup()

	if path == "" {
		t.Fatal("expected temp audio path to be returned")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected saved audio file to contain data")
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected cleanup to remove temp file, got err=%v", err)
	}
}
