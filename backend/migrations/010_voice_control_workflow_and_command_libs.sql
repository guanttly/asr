ALTER TABLE user_workflow_bindings
  ADD COLUMN voice_control_workflow_id BIGINT UNSIGNED NULL AFTER meeting_workflow_id,
  ADD CONSTRAINT fk_user_workflow_bindings_voice_control FOREIGN KEY (voice_control_workflow_id) REFERENCES workflows(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS voice_command_dicts (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  name VARCHAR(128) NOT NULL,
  group_key VARCHAR(64) NOT NULL,
  description TEXT NULL,
  is_base TINYINT(1) NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  UNIQUE KEY uk_voice_command_dicts_group_key (group_key),
  KEY idx_voice_command_dicts_is_base (is_base)
);

CREATE TABLE IF NOT EXISTS voice_command_entries (
  id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  dict_id BIGINT UNSIGNED NOT NULL,
  intent VARCHAR(128) NOT NULL,
  label VARCHAR(128) NOT NULL,
  utterances_json JSON NOT NULL,
  enabled TINYINT(1) NOT NULL DEFAULT 1,
  sort_order INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (id),
  KEY idx_voice_command_entries_dict_id (dict_id),
  KEY idx_voice_command_entries_intent (intent),
  CONSTRAINT fk_voice_command_entries_dict FOREIGN KEY (dict_id) REFERENCES voice_command_dicts(id) ON DELETE CASCADE
);