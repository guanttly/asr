UPDATE transcription_tasks
SET language = 'auto'
WHERE language IS NULL OR language = '';

ALTER TABLE transcription_tasks
  MODIFY COLUMN language VARCHAR(16) NOT NULL DEFAULT 'auto';

ALTER TABLE meetings
  ADD COLUMN language VARCHAR(16) NOT NULL DEFAULT 'auto' AFTER duration;