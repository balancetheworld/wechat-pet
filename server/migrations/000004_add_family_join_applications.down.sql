DROP TABLE IF EXISTS family_join_applications;
DROP INDEX IF EXISTS idx_families_code;
ALTER TABLE families DROP COLUMN code;
