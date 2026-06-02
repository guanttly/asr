-- 015_keep_only_imaging_default_term_dict.sql
-- Historical note:
-- The product now ships only the "影像报告" default terminology dictionary, but
-- upgrade scripts must not delete older dictionaries by name. Operators may
-- have customized those dictionaries after installation.
--
-- This file is intentionally non-destructive. Use the result set below for
-- manual review after taking a database backup.

SELECT id, name, domain
FROM term_dicts
WHERE (name = '写报告' AND domain = '医疗')
   OR (name = '医疗查房' AND domain = '医疗')
   OR (name = '检验报告' AND domain = '医疗检验');
