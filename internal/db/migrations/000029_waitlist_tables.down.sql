DROP INDEX IF EXISTS idx_waitlist_offers_waitlist_id;
DROP INDEX IF EXISTS idx_waitlists_status;
DROP INDEX IF EXISTS idx_waitlists_target_date;
DROP INDEX IF EXISTS idx_waitlists_user_id;
DROP INDEX IF EXISTS idx_waitlists_slot;
DROP INDEX IF EXISTS idx_waitlists_facility_id;

DROP TABLE IF EXISTS waitlist_offers;
DROP TABLE IF EXISTS waitlists;
DROP TABLE IF EXISTS waitlist_config;
