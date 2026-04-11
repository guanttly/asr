package asr

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lgt/asr/internal/infrastructure/audiofile"
)

const defaultStreamSessionTTL = 15 * time.Minute
const streamSessionSampleRate = 16000

var ErrStreamSessionNotFound = errors.New("stream session not found")
var ErrStreamSessionExpired = errors.New("stream session expired")
var ErrStreamSessionActive = errors.New("stream session is still active")
var ErrStreamSessionClosed = errors.New("stream session already finalized")
var ErrStreamSessionEmptyAudio = errors.New("stream session contains no audio")

type managedStreamSession struct {
	mu                sync.Mutex
	upstreamSessionID string
	transcriptText    string
	committedText     string
	pcmData           []byte
	audioFilePath     string
	durationSeconds   float64
	finalized         bool
	language          string
	expiresAt         time.Time
}

func (s *Service) SetStreamSessionTTL(ttl time.Duration) {
	if s == nil || ttl <= 0 {
		return
	}
	s.streamSessionTTL = ttl
}

func (s *Service) newManagedStreamSession(upstreamSessionID string, now time.Time) *managedStreamSession {
	return &managedStreamSession{
		upstreamSessionID: strings.TrimSpace(upstreamSessionID),
		expiresAt:         now.Add(s.streamSessionTTL),
	}
}

func (s *Service) newStreamSessionID() string {
	if s != nil && s.streamSessionIDFn != nil {
		return s.streamSessionIDFn()
	}
	return generateRandomStreamSessionID()
}

func generateRandomStreamSessionID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("stream-%d", time.Now().UnixNano())
}

func (s *Service) cleanupExpiredStreamSessions(now time.Time) {
	if s == nil {
		return
	}

	s.streamSessions.Range(func(key, value any) bool {
		sessionID, ok := key.(string)
		if !ok {
			return true
		}
		session, ok := value.(*managedStreamSession)
		if !ok || session == nil {
			s.streamSessions.Delete(key)
			return true
		}

		session.mu.Lock()
		expired := !session.expiresAt.IsZero() && !now.Before(session.expiresAt)
		upstreamSessionID := session.upstreamSessionID
		audioFilePath := session.audioFilePath
		finalized := session.finalized
		session.mu.Unlock()
		if !expired {
			return true
		}

		s.streamSessions.Delete(sessionID)
		if audioFilePath != "" {
			_ = os.Remove(audioFilePath)
		}
		if !finalized && s.streamingEngine != nil && strings.TrimSpace(upstreamSessionID) != "" {
			go func(upstreamID string) {
				_, _ = s.streamingEngine.FinishStreamSession(context.Background(), upstreamID)
			}(upstreamSessionID)
		}
		return true
	})
}

func (s *Service) loadManagedStreamSession(sessionID string, now time.Time) (*managedStreamSession, error) {
	s.cleanupExpiredStreamSessions(now)
	value, ok := s.streamSessions.Load(strings.TrimSpace(sessionID))
	if !ok {
		return nil, ErrStreamSessionNotFound
	}

	session, ok := value.(*managedStreamSession)
	if !ok || session == nil {
		s.streamSessions.Delete(strings.TrimSpace(sessionID))
		return nil, ErrStreamSessionNotFound
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.expiresAt.IsZero() && !now.Before(session.expiresAt) {
		upstreamSessionID := session.upstreamSessionID
		audioFilePath := session.audioFilePath
		finalized := session.finalized
		s.streamSessions.Delete(strings.TrimSpace(sessionID))
		if audioFilePath != "" {
			_ = os.Remove(audioFilePath)
		}
		if !finalized && s.streamingEngine != nil && strings.TrimSpace(upstreamSessionID) != "" {
			go func(upstreamID string) {
				_, _ = s.streamingEngine.FinishStreamSession(context.Background(), upstreamID)
			}(upstreamSessionID)
		}
		return nil, ErrStreamSessionExpired
	}

	return session, nil
}

func pcmBytesToDurationSeconds(pcmData []byte) float64 {
	if len(pcmData) == 0 {
		return 0
	}
	return float64(len(pcmData)) / 2 / streamSessionSampleRate
}

func (s *Service) materializeManagedStreamAudio(sessionID string, session *managedStreamSession) (string, float64, error) {
	if session.audioFilePath != "" {
		return session.audioFilePath, session.durationSeconds, nil
	}
	if len(session.pcmData) == 0 {
		return "", 0, ErrStreamSessionEmptyAudio
	}

	outputPath := filepath.Join(os.TempDir(), fmt.Sprintf("asr-stream-session-%s.wav", sessionID))
	if err := audiofile.WritePCM16MonoWAV(outputPath, session.pcmData, streamSessionSampleRate); err != nil {
		return "", 0, err
	}
	session.audioFilePath = outputPath
	session.durationSeconds = pcmBytesToDurationSeconds(session.pcmData)
	return outputPath, session.durationSeconds, nil
}

func (s *Service) consumeManagedStreamAudio(sessionID string) (string, float64, error) {
	now := time.Now()
	session, err := s.loadManagedStreamSession(sessionID, now)
	if err != nil {
		return "", 0, err
	}

	session.mu.Lock()
	defer session.mu.Unlock()
	if !session.finalized {
		return "", 0, ErrStreamSessionActive
	}
	path, duration, err := s.materializeManagedStreamAudio(sessionID, session)
	if err != nil {
		return "", 0, err
	}
	s.streamSessions.Delete(strings.TrimSpace(sessionID))
	session.pcmData = nil
	return path, duration, nil
}

func (s *Service) applyStreamChunkResult(sessionID string, session *managedStreamSession, result *StreamChunkResponse, now time.Time, isFinal bool) *StreamChunkResponse {
	current := strings.TrimSpace(session.transcriptText)
	incoming := sanitizeTranscriptionText(result.Text)
	merged := mergeStreamingTranscript(current, incoming)
	delta := extractStreamingTranscriptDelta(current, merged)

	session.transcriptText = merged
	if language := strings.TrimSpace(result.Language); language != "" {
		session.language = language
	}
	if !isFinal {
		session.expiresAt = now.Add(s.streamSessionTTL)
	}

	return &StreamChunkResponse{
		SessionID: sessionID,
		Language:  session.language,
		Text:      merged,
		TextDelta: delta,
		IsFinal:   isFinal,
	}
}

func (s *Service) applyStreamCommitResult(sessionID string, session *managedStreamSession, now time.Time, isFinal bool) *StreamChunkResponse {
	committed := strings.TrimSpace(session.committedText)
	latest := strings.TrimSpace(session.transcriptText)
	delta := extractStreamingTranscriptDelta(committed, latest)
	session.committedText = latest
	if !isFinal {
		session.expiresAt = now.Add(s.streamSessionTTL)
	}

	return &StreamChunkResponse{
		SessionID: sessionID,
		Language:  session.language,
		Text:      latest,
		TextDelta: delta,
		IsFinal:   isFinal,
	}
}

func mergeStreamingTranscript(current, incoming string) string {
	trimmedIncoming := strings.TrimSpace(incoming)
	if trimmedIncoming == "" {
		return strings.TrimSpace(current)
	}

	trimmedCurrent := strings.TrimSpace(current)
	if trimmedCurrent == "" {
		return trimmedIncoming
	}
	if trimmedIncoming == trimmedCurrent {
		return trimmedCurrent
	}
	if strings.Contains(trimmedIncoming, trimmedCurrent) {
		return trimmedIncoming
	}
	if strings.Contains(trimmedCurrent, trimmedIncoming) {
		return trimmedCurrent
	}

	overlap := longestSuffixPrefixLength(trimmedCurrent, trimmedIncoming)
	if overlap > 0 {
		return strings.TrimSpace(trimmedCurrent + trimmedIncoming[overlap:])
	}

	commonPrefix := longestCommonPrefixLength(trimmedCurrent, trimmedIncoming)
	if commonPrefix >= int(float64(minInt(len(trimmedCurrent), len(trimmedIncoming)))*0.7) {
		if len(trimmedIncoming) >= len(trimmedCurrent) {
			return trimmedIncoming
		}
		return trimmedCurrent
	}

	return strings.TrimSpace(trimmedCurrent + trimmedIncoming)
}

func extractStreamingTranscriptDelta(current, latest string) string {
	trimmedCurrent := strings.TrimSpace(current)
	trimmedLatest := strings.TrimSpace(latest)
	if trimmedLatest == "" || trimmedLatest == trimmedCurrent {
		return ""
	}
	if trimmedCurrent == "" {
		return trimmedLatest
	}
	if strings.HasPrefix(trimmedLatest, trimmedCurrent) {
		return strings.TrimSpace(trimmedLatest[len(trimmedCurrent):])
	}

	overlap := longestSuffixPrefixLength(trimmedCurrent, trimmedLatest)
	if overlap > 0 {
		return strings.TrimSpace(trimmedLatest[overlap:])
	}

	commonPrefix := longestCommonPrefixLength(trimmedCurrent, trimmedLatest)
	if commonPrefix >= int(float64(minInt(len(trimmedCurrent), len(trimmedLatest)))*0.7) {
		return strings.TrimSpace(trimmedLatest[commonPrefix:])
	}

	return trimmedLatest
}

func longestCommonPrefixLength(left, right string) int {
	limit := minInt(len(left), len(right))
	for index := 0; index < limit; index++ {
		if left[index] != right[index] {
			return index
		}
	}
	return limit
}

func longestSuffixPrefixLength(left, right string) int {
	limit := minInt(len(left), len(right))
	for size := limit; size > 0; size-- {
		if left[len(left)-size:] == right[:size] {
			return size
		}
	}
	return 0
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
