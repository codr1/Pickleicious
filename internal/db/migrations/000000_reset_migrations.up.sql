CREATE TABLE IF NOT EXISTS schema_migrations (
    version bigint not null primary key,
    dirty boolean not null
); 