-- internal/db/migrations/000003_add_member_photos.up.sql

-- TODO: Future enhancement - move to S3-compatible storage like Minio
-- For now storing in DB for portability and simplicity
CREATE TABLE member_photos (
    id INTEGER PRIMARY KEY,
    member_id INTEGER NOT NULL,
    data BLOB NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (member_id) REFERENCES members(id)
);

-- Only keep one photo per member
CREATE UNIQUE INDEX idx_member_photos_member_id ON member_photos(member_id);
