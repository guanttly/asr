-- 014_remove_legacy_nonmedical_seeds.sql
-- Historical note:
-- Earlier builds used this migration as an operator helper to delete the
-- factory "庭审记录"/"会议纪要" terminology dictionaries. That is unsafe during
-- upgrades because the schema cannot distinguish untouched factory data from
-- user-customized dictionaries with the same name.
--
-- This file is intentionally non-destructive. If cleanup is needed, first
-- export/backup the database, inspect matching rows manually, and delete only
-- records confirmed to be disposable factory data.

SELECT id, name, domain
FROM term_dicts
WHERE (name = '庭审记录' AND domain = '法律')
   OR (name = '会议纪要' AND domain = '办公');
