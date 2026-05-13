ALTER TABLE correction_rules
  ADD COLUMN priority INT NOT NULL DEFAULT 100 AFTER sort_order,
  ADD COLUMN conflict_group VARCHAR(128) NULL AFTER priority;

UPDATE correction_rules
SET priority = sort_order
WHERE priority IS NULL OR priority <= 0;