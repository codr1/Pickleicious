PRAGMA foreign_keys = ON;

CREATE TABLE reservation_cancellations (
    id INTEGER PRIMARY KEY,
    reservation_id INTEGER NOT NULL,
    cancelled_by_user_id INTEGER NOT NULL,
    cancelled_at DATETIME NOT NULL,
    refund_percentage_applied INTEGER NOT NULL,
    fee_waived BOOLEAN NOT NULL DEFAULT 0,
    hours_before_start INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (refund_percentage_applied >= 0 AND refund_percentage_applied <= 100),
    CHECK (hours_before_start >= 0),
    FOREIGN KEY (reservation_id) REFERENCES reservations(id),
    FOREIGN KEY (cancelled_by_user_id) REFERENCES users(id)
);

CREATE INDEX idx_reservation_cancellations_reservation_id ON reservation_cancellations(reservation_id);
CREATE INDEX idx_reservation_cancellations_cancelled_by_user_id ON reservation_cancellations(cancelled_by_user_id);
CREATE INDEX idx_reservation_cancellations_cancelled_at ON reservation_cancellations(cancelled_at);
