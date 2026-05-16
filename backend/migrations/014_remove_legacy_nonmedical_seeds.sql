-- 014_remove_legacy_nonmedical_seeds.sql
-- 删除产品聚焦医学领域前内置的"庭审记录"与"会议纪要"种子词典及其所有条目/规则。
-- 注:实际清理也由 backend/internal/application/terminology/service.go::removeLegacyNonMedicalSeeds
-- 在启动时执行一次。本 SQL 仅作为离线/手动运维入口。

DELETE FROM correction_rules
WHERE dict_id IN (
  SELECT id FROM term_dicts
  WHERE (name = '庭审记录' AND domain = '法律')
     OR (name = '会议纪要' AND domain = '办公')
);

DELETE FROM term_entries
WHERE dict_id IN (
  SELECT id FROM term_dicts
  WHERE (name = '庭审记录' AND domain = '法律')
     OR (name = '会议纪要' AND domain = '办公')
);

DELETE FROM term_dicts
WHERE (name = '庭审记录' AND domain = '法律')
   OR (name = '会议纪要' AND domain = '办公');
