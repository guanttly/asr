ALTER TABLE workflows
  ADD COLUMN IF NOT EXISTS workflow_type VARCHAR(50) NOT NULL DEFAULT 'legacy' AFTER description,
  ADD COLUMN IF NOT EXISTS source_kind VARCHAR(50) NOT NULL DEFAULT 'legacy_text' AFTER workflow_type,
  ADD COLUMN IF NOT EXISTS target_kind VARCHAR(50) NOT NULL DEFAULT 'transcript' AFTER source_kind,
  ADD COLUMN IF NOT EXISTS is_legacy TINYINT(1) NOT NULL DEFAULT 1 AFTER target_kind,
  ADD COLUMN IF NOT EXISTS validation_message TEXT NULL AFTER is_legacy,
  ADD INDEX IF NOT EXISTS idx_workflow_type (workflow_type),
  ADD INDEX IF NOT EXISTS idx_workflow_source_kind (source_kind),
  ADD INDEX IF NOT EXISTS idx_workflow_target_kind (target_kind),
  ADD INDEX IF NOT EXISTS idx_workflow_is_legacy (is_legacy);