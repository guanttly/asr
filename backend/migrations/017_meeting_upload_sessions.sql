-- Resumable, crash-safe meeting upload sessions.
--
-- A long meeting recording is streamed to the server progressively as raw-PCM
-- segments. Each segment is persisted and tracked so an in-progress upload
-- survives a backend restart and can be resumed (or recovered server-side)
-- after a client crash or network drop.
--
-- NOTE: GORM AutoMigrate is the primary schema mechanism; this file exists for
-- parity / manual provisioning and is safe to run repeatedly.

CREATE TABLE IF NOT EXISTS meeting_upload_sessions (
  id              BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  upload_id       VARCHAR(64)     NOT NULL,
  user_id         BIGINT UNSIGNED NOT NULL,
  meeting_id      BIGINT UNSIGNED NULL,
  status          VARCHAR(20)     NOT NULL DEFAULT 'recording',
  format          VARCHAR(32)     NOT NULL DEFAULT 'pcm_s16le_16000_mono',
  filename        VARCHAR(255)    NULL,
  title           VARCHAR(255)    NULL,
  workflow_id     BIGINT UNSIGNED NULL,
  language        VARCHAR(16)     NOT NULL DEFAULT 'auto',
  duration_sec    DOUBLE          NOT NULL DEFAULT 0,
  total_bytes     BIGINT          NOT NULL DEFAULT 0,
  next_index      INT             NOT NULL DEFAULT 0,
  public_base_url VARCHAR(512)    NULL,
  started_at      DATETIME(3)     NULL,
  last_seen_at    DATETIME(3)     NULL,
  stopped_at      DATETIME(3)     NULL,
  completed_at    DATETIME(3)     NULL,
  aborted_at      DATETIME(3)     NULL,
  created_at      DATETIME(3)     NULL,
  updated_at      DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_meeting_upload_sessions_upload_id (upload_id),
  KEY idx_meeting_upload_sessions_user_id (user_id),
  KEY idx_meeting_upload_sessions_meeting_id (meeting_id),
  KEY idx_meeting_upload_sessions_status (status),
  KEY idx_meeting_upload_sessions_last_seen_at (last_seen_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS meeting_upload_segments (
  id                BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  upload_session_id BIGINT UNSIGNED NOT NULL,
  segment_index     INT             NOT NULL,
  path              VARCHAR(1024)   NOT NULL,
  bytes             BIGINT          NOT NULL DEFAULT 0,
  duration_sec      DOUBLE          NULL,
  checksum          VARCHAR(128)    NULL,
  status            VARCHAR(16)     NOT NULL DEFAULT 'stored',
  created_at        DATETIME(3)     NULL,
  updated_at        DATETIME(3)     NULL,
  PRIMARY KEY (id),
  UNIQUE KEY uk_meeting_upload_segment (upload_session_id, segment_index)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Link a meeting back to the upload session that produced it and track audio
-- archival for retention/cleanup.
ALTER TABLE meetings
  ADD COLUMN upload_session_id BIGINT UNSIGNED NULL AFTER source_task_id,
  ADD COLUMN archived_at DATETIME(3) NULL AFTER next_sync_at,
  ADD KEY idx_meetings_upload_session_id (upload_session_id);
