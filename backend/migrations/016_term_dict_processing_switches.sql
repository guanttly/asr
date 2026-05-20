ALTER TABLE term_dicts
  ADD COLUMN rule_processing_enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER domain,
  ADD COLUMN text_replacement_enabled TINYINT(1) NOT NULL DEFAULT 1 AFTER rule_processing_enabled;