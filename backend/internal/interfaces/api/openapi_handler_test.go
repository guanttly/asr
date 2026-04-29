package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	appasr "github.com/lgt/asr/internal/application/asr"
	openplatformdomain "github.com/lgt/asr/internal/domain/openplatform"
)

func TestBuildOpenTranscriptPayloadSplitsSentencesForVAD(t *testing.T) {
	segments := buildOpenTranscriptPayload("第一句。第二句！第三句", 6, true)
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %+v", segments)
	}
	if segments[0]["text"] != "第一句。" {
		t.Fatalf("unexpected first segment: %+v", segments[0])
	}
	if segments[2]["end_ms"] != 6000 {
		t.Fatalf("expected final segment end at 6000ms, got %+v", segments[2])
	}
}

func TestOpenAPIStreamSessionEventsAndCommit(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	batchEngine := &batchEngineHandlerStub{
		streamSessionID:    "upstream-stream-1",
		streamChunkResult:  &appasr.StreamChunkResponse{SessionID: "upstream-stream-1", Text: "你好", Language: "zh"},
		streamFinishResult: &appasr.StreamChunkResponse{SessionID: "upstream-stream-1", Text: "你好世界", Language: "zh"},
	}
	handler := NewOpenAPIASRHandler(appasr.NewService(nil, batchEngine, nil, 5, nil), nil, nil, "uploads", "", 100)

	router := gin.New()
	group := router.Group("/openapi/v1/asr")
	group.Use(func(c *gin.Context) {
		c.Set("open_auth_app", &openplatformdomain.App{ID: 1, AppID: "app_test"})
		c.Set("open_request_id", "req_test")
		c.Next()
	})
	handler.Register(group)

	server := httptest.NewServer(router)
	defer server.Close()

	startReq, err := http.NewRequest(http.MethodPost, server.URL+"/openapi/v1/asr/stream-sessions?access_token=test-token", nil)
	if err != nil {
		t.Fatalf("create start request: %v", err)
	}
	startResp, err := http.DefaultClient.Do(startReq)
	if err != nil {
		t.Fatalf("start stream session request failed: %v", err)
	}
	defer startResp.Body.Close()
	if startResp.StatusCode != http.StatusOK {
		t.Fatalf("expected start status 200, got %d", startResp.StatusCode)
	}
	var startEnvelope responseEnvelope[map[string]any]
	if err := json.NewDecoder(startResp.Body).Decode(&startEnvelope); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	sessionID, _ := startEnvelope.Data["session_id"].(string)
	wsURL, _ := startEnvelope.Data["ws_url"].(string)
	commitURL, _ := startEnvelope.Data["commit_url"].(string)
	if sessionID == "" || wsURL == "" || commitURL == "" {
		t.Fatalf("expected session and stream urls, got %+v", startEnvelope.Data)
	}
	if !strings.Contains(wsURL, "access_token=test-token") {
		t.Fatalf("expected ws_url to include access_token, got %s", wsURL)
	}

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws_url: %v", err)
	}
	defer conn.Close()

	readyEvent := readOpenStreamEvent(t, conn)
	if readyEvent["type"] != "session.ready" {
		t.Fatalf("expected ready event, got %+v", readyEvent)
	}

	chunkReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/openapi/v1/asr/stream-sessions/%s/chunks?access_token=test-token", server.URL, sessionID), bytes.NewReader([]byte{1, 2, 3, 4}))
	if err != nil {
		t.Fatalf("create chunk request: %v", err)
	}
	chunkReq.Header.Set("Content-Type", "application/octet-stream")
	chunkResp, err := http.DefaultClient.Do(chunkReq)
	if err != nil {
		t.Fatalf("push chunk request failed: %v", err)
	}
	chunkResp.Body.Close()
	if chunkResp.StatusCode != http.StatusOK {
		t.Fatalf("expected chunk status 200, got %d", chunkResp.StatusCode)
	}

	partialEvent := readOpenStreamEventUntilType(t, conn, "transcript.partial")
	if partialEvent["text"] != "你好" || partialEvent["text_delta"] != "你好" {
		t.Fatalf("unexpected partial event: %+v", partialEvent)
	}

	commitReq, err := http.NewRequest(http.MethodPost, commitURL, nil)
	if err != nil {
		t.Fatalf("create commit request: %v", err)
	}
	commitResp, err := http.DefaultClient.Do(commitReq)
	if err != nil {
		t.Fatalf("commit request failed: %v", err)
	}
	defer commitResp.Body.Close()
	if commitResp.StatusCode != http.StatusOK {
		t.Fatalf("expected commit status 200, got %d", commitResp.StatusCode)
	}
	var commitEnvelope responseEnvelope[map[string]any]
	if err := json.NewDecoder(commitResp.Body).Decode(&commitEnvelope); err != nil {
		t.Fatalf("decode commit response: %v", err)
	}
	if commitEnvelope.Data["text_delta"] != "你好" {
		t.Fatalf("unexpected commit response: %+v", commitEnvelope.Data)
	}

	segmentEvent := readOpenStreamEventUntilType(t, conn, "transcript.segment")
	if segmentEvent["segment_text"] != "你好" {
		t.Fatalf("unexpected segment event: %+v", segmentEvent)
	}

	finishReq, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/openapi/v1/asr/stream-sessions/%s/finish?access_token=test-token", server.URL, sessionID), nil)
	if err != nil {
		t.Fatalf("create finish request: %v", err)
	}
	finishResp, err := http.DefaultClient.Do(finishReq)
	if err != nil {
		t.Fatalf("finish request failed: %v", err)
	}
	finishResp.Body.Close()
	if finishResp.StatusCode != http.StatusOK {
		t.Fatalf("expected finish status 200, got %d", finishResp.StatusCode)
	}

	var sawUpdatedPartial bool
	for index := 0; index < 4; index++ {
		event := readOpenStreamEvent(t, conn)
		if event["type"] == "transcript.partial" && event["text_delta"] == "世界" {
			sawUpdatedPartial = true
		}
		if event["type"] == "session.finished" {
			if !sawUpdatedPartial {
				t.Fatalf("expected updated partial event before finished event")
			}
			if event["text"] != "你好世界" {
				t.Fatalf("unexpected finished event: %+v", event)
			}
			return
		}
	}
	t.Fatal("expected session.finished event")
}

func readOpenStreamEventUntilType(t *testing.T, conn *websocket.Conn, expectedType string) map[string]any {
	t.Helper()
	for index := 0; index < 6; index++ {
		event := readOpenStreamEvent(t, conn)
		if event["type"] == expectedType {
			return event
		}
	}
	t.Fatalf("expected websocket event type %s", expectedType)
	return nil
}

func readOpenStreamEvent(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read websocket message: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode websocket message %s: %v", string(payload), err)
	}
	return event
}
