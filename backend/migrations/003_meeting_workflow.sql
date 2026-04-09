ALTER TABLE meetings
  ADD COLUMN workflow_id BIGINT UNSIGNED NULL AFTER source_task_id,
  ADD KEY idx_meetings_workflow_id (workflow_id);
