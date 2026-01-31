CREATE INDEX idx_reservations_facility_id_start_time
    ON reservations(facility_id, start_time);
