package meetingupload

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	appmeeting "github.com/lgt/asr/internal/application/meeting"
	domain "github.com/lgt/asr/internal/domain/meetingupload"
)

// Fixed raw-PCM audio parameters streamed by the desktop client: 16-bit signed
// little-endian, 16 kHz, mono. Duration is derived deterministically from the
// byte count, so no media probing is required when assembling the audio.
const (
	audioSampleRate   = 16000
	audioBytesPerSamp = 2
	audioChannels     = 1
	audioByteRate     = audioSampleRate * audioBytesPerSamp * audioChannels
)

const lockShards = 64

// Config tunes the resumable upload service.
type Config struct {
	UploadDir              string
	SegmentsSubdir         string
	AudioSubdir            string
	MinRecordingDuration   time.Duration
	InactiveTimeout        time.Duration
	ResumeRetention        time.Duration
	CompletedTempRetention time.Duration
	AbortedRetention       time.Duration
	MaxChunkBytes          int64
	MaxSessionBytes        int64
	RecoverBatchSize       int
	CleanupBatchSize       int
}

func (c Config) withDefaults() Config {
	if c.SegmentsSubdir == "" {
		c.SegmentsSubdir = "meeting-uploads"
	}
	if c.AudioSubdir == "" {
		c.AudioSubdir = "audio"
	}
	if c.MinRecordingDuration <= 0 {
		c.MinRecordingDuration = 5 * time.Second
	}
	if c.InactiveTimeout <= 0 {
		c.InactiveTimeout = time.Hour
	}
	if c.ResumeRetention <= 0 {
		c.ResumeRetention = 7 * 24 * time.Hour
	}
	if c.CompletedTempRetention <= 0 {
		c.CompletedTempRetention = 24 * time.Hour
	}
	if c.AbortedRetention <= 0 {
		c.AbortedRetention = time.Hour
	}
	if c.MaxChunkBytes <= 0 {
		c.MaxChunkBytes = 8 * 1024 * 1024
	}
	if c.MaxSessionBytes <= 0 {
		c.MaxSessionBytes = 4096 * 1024 * 1024
	}
	if c.RecoverBatchSize <= 0 {
		c.RecoverBatchSize = 50
	}
	if c.CleanupBatchSize <= 0 {
		c.CleanupBatchSize = 100
	}
	return c
}

// MeetingFinalizer is the subset of the meeting application service the upload
// pipeline depends on. Keeping it as an interface avoids an import cycle and
// lets the upload service stay focused on durable storage.
type MeetingFinalizer interface {
	CreateUploadingMeeting(ctx context.Context, p appmeeting.UploadingMeetingParams) (uint64, error)
	FinalizeUploadedMeeting(ctx context.Context, p appmeeting.FinalizeUploadedParams) error
	CreateUploadedMeeting(ctx context.Context, p appmeeting.UploadedMeetingParams) (uint64, error)
	MarkMeetingInterrupted(ctx context.Context, meetingID uint64) error
	DiscardMeeting(ctx context.Context, meetingID uint64) error
}

// Logger is the minimal logging surface used by the service.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

type nopLogger struct{}

func (nopLogger) Infof(string, ...any)  {}
func (nopLogger) Warnf(string, ...any)  {}
func (nopLogger) Errorf(string, ...any) {}

// Service orchestrates resumable, crash-safe meeting uploads.
type Service struct {
	repo      domain.Repository
	finalizer MeetingFinalizer
	cfg       Config
	logger    Logger

	locks [lockShards]sync.Mutex
}

// NewService creates an upload service.
func NewService(repo domain.Repository, finalizer MeetingFinalizer, cfg Config, logger Logger) *Service {
	if logger == nil {
		logger = nopLogger{}
	}
	return &Service{repo: repo, finalizer: finalizer, cfg: cfg.withDefaults(), logger: logger}
}

// Error is a typed upload error carrying an HTTP status for the API layer.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

func newError(status int, message string) *Error { return &Error{Status: status, Message: message} }

// MissingChunksError is returned by Complete when the assembled stream has gaps.
type MissingChunksError struct {
	Missing []int
}

func (e *MissingChunksError) Error() string {
	return fmt.Sprintf("missing %d chunk(s)", len(e.Missing))
}

func (s *Service) lockFor(uploadID string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(uploadID))
	return &s.locks[h.Sum32()%lockShards]
}

func durationFromBytes(b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(b) / float64(audioByteRate)
}

func newUploadID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("upload-%d", time.Now().UnixNano())
}

// buildAudioURL builds an absolute URL to an assembled audio file from the base
// URL captured at init time (no request context available in background tasks).
func buildAudioURL(publicBaseURL, relativePath string) (string, error) {
	base := publicBaseURL
	if base == "" {
		return "", newError(http.StatusInternalServerError, "missing public base url for assembled audio")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", newError(http.StatusInternalServerError, "invalid public base url")
	}
	parsed.Path = path.Join(parsed.Path, "/uploads", relativePath)
	return parsed.String(), nil
}
