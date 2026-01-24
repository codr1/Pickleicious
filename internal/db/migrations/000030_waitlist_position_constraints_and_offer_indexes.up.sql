CREATE UNIQUE INDEX IF NOT EXISTS idx_waitlists_slot_position_unique
    ON waitlists(facility_id, target_date, target_start_time, target_end_time, COALESCE(target_court_id, -1), position);

CREATE INDEX IF NOT EXISTS idx_waitlist_offers_status
    ON waitlist_offers(status);

CREATE INDEX IF NOT EXISTS idx_waitlist_offers_expires_at
    ON waitlist_offers(expires_at);
