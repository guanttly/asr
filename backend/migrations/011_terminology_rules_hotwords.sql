ALTER TABLE correction_rules
  ADD COLUMN match_type VARCHAR(32) NOT NULL DEFAULT 'literal' AFTER layer,
  ADD COLUMN sort_order INT NOT NULL DEFAULT 100 AFTER enabled;

UPDATE correction_rules
SET match_type = 'literal'
WHERE match_type IS NULL OR match_type = '';

ALTER TABLE transcription_tasks
  ADD COLUMN language VARCHAR(16) NOT NULL DEFAULT 'auto' AFTER workflow_id,
  ADD COLUMN use_itn TINYINT(1) NULL AFTER language,
  ADD COLUMN hotwords_json JSON NULL AFTER use_itn;

ALTER TABLE term_entries DROP COLUMN pinyin;

ALTER TABLE correction_rules DROP COLUMN layer;
