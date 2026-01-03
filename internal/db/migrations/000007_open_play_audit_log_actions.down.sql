PRAGMA foreign_keys = OFF;

ALTER TABLE open_play_audit_log RENAME TO open_play_audit_log_old;

CREATE TABLE open_play_audit_log (
    id INTEGER PRIMARY KEY,
    session_id INTEGER NOT NULL,
    action TEXT NOT NULL,
    before_state TEXT,
    after_state TEXT,
    reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (action IN ('scale_up', 'scale_down', 'cancelled')),
    FOREIGN KEY (session_id) REFERENCES open_play_sessions(id)
);

DELETE FROM open_play_audit_log_old
WHERE action IN ('auto_scale_override', 'auto_scale_rule_disabled');

INSERT INTO open_play_audit_log (
    id,
    session_id,
    action,
    before_state,
    after_state,
    reason,
    created_at
)
SELECT
    id,
    session_id,
    action,
    before_state,
    after_state,
    reason,
    created_at
FROM open_play_audit_log_old;

DROP TABLE open_play_audit_log_old;

CREATE INDEX idx_open_play_audit_log_session_id ON open_play_audit_log(session_id);
CREATE INDEX idx_open_play_audit_log_created_at ON open_play_audit_log(created_at);

PRAGMA foreign_keys = ON;
