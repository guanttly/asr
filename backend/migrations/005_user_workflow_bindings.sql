CREATE TABLE IF NOT EXISTS user_workflow_bindings (
  user_id BIGINT UNSIGNED NOT NULL,
  realtime_workflow_id BIGINT UNSIGNED NULL,
  batch_workflow_id BIGINT UNSIGNED NULL,
  meeting_workflow_id BIGINT UNSIGNED NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (user_id),
  CONSTRAINT fk_user_workflow_bindings_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_user_workflow_bindings_realtime FOREIGN KEY (realtime_workflow_id) REFERENCES workflows(id) ON DELETE SET NULL,
  CONSTRAINT fk_user_workflow_bindings_batch FOREIGN KEY (batch_workflow_id) REFERENCES workflows(id) ON DELETE SET NULL,
  CONSTRAINT fk_user_workflow_bindings_meeting FOREIGN KEY (meeting_workflow_id) REFERENCES workflows(id) ON DELETE SET NULL
);