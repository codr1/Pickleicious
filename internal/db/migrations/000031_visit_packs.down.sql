PRAGMA foreign_keys = OFF;

DROP TRIGGER IF EXISTS visit_pack_types_limit_update;
DROP TRIGGER IF EXISTS visit_pack_types_limit_insert;

DROP INDEX IF EXISTS idx_visit_pack_redemptions_redeemed_at;
DROP INDEX IF EXISTS idx_visit_pack_redemptions_reservation_id;
DROP INDEX IF EXISTS idx_visit_pack_redemptions_facility_id;
DROP INDEX IF EXISTS idx_visit_pack_redemptions_visit_pack_id;
DROP TABLE IF EXISTS visit_pack_redemptions;

DROP INDEX IF EXISTS idx_visit_packs_user_status_expires;
DROP INDEX IF EXISTS idx_visit_packs_expires_at;
DROP INDEX IF EXISTS idx_visit_packs_status;
DROP INDEX IF EXISTS idx_visit_packs_user_id;
DROP INDEX IF EXISTS idx_visit_packs_pack_type_id;
DROP TABLE IF EXISTS visit_packs;

DROP INDEX IF EXISTS idx_visit_pack_types_status;
DROP INDEX IF EXISTS idx_visit_pack_types_facility_id;
DROP TABLE IF EXISTS visit_pack_types;

ALTER TABLE organizations DROP COLUMN cross_facility_visit_packs;

PRAGMA foreign_keys = ON;
