-- 015_keep_only_imaging_default_term_dict.sql
-- 默认术语库仅保留「影像报告」。清理此前随系统内置的通用/查房/检验默认词库。

DELETE FROM correction_rules
WHERE dict_id IN (
  SELECT id FROM term_dicts
  WHERE (name = '写报告' AND domain = '医疗')
     OR (name = '医疗查房' AND domain = '医疗')
     OR (name = '检验报告' AND domain = '医疗检验')
);

DELETE FROM term_entries
WHERE dict_id IN (
  SELECT id FROM term_dicts
  WHERE (name = '写报告' AND domain = '医疗')
     OR (name = '医疗查房' AND domain = '医疗')
     OR (name = '检验报告' AND domain = '医疗检验')
);

DELETE FROM term_dicts
WHERE (name = '写报告' AND domain = '医疗')
   OR (name = '医疗查房' AND domain = '医疗')
   OR (name = '检验报告' AND domain = '医疗检验');