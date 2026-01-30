PRAGMA foreign_keys = OFF;

DROP TRIGGER IF EXISTS lesson_package_types_limit_update;
DROP TRIGGER IF EXISTS lesson_package_types_limit_insert;

DROP INDEX IF EXISTS idx_lesson_package_redemptions_redeemed_at;
DROP INDEX IF EXISTS idx_lesson_package_redemptions_reservation_id;
DROP INDEX IF EXISTS idx_lesson_package_redemptions_facility_id;
DROP INDEX IF EXISTS idx_lesson_package_redemptions_lesson_package_id;
DROP TABLE IF EXISTS lesson_package_redemptions;

DROP INDEX IF EXISTS idx_lesson_packages_user_status_expires;
DROP INDEX IF EXISTS idx_lesson_packages_expires_at;
DROP INDEX IF EXISTS idx_lesson_packages_status;
DROP INDEX IF EXISTS idx_lesson_packages_user_id;
DROP INDEX IF EXISTS idx_lesson_packages_pack_type_id;
DROP TABLE IF EXISTS lesson_packages;

DROP INDEX IF EXISTS idx_lesson_package_types_status;
DROP INDEX IF EXISTS idx_lesson_package_types_facility_id;
DROP TABLE IF EXISTS lesson_package_types;

PRAGMA foreign_keys = ON;
