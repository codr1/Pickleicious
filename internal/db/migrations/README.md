// internal/db/migrations/README.md
# Database Migrations

This directory contains database migrations for the Pickleicious application.
Migrations are handled using golang-migrate.

## Migration Naming Convention

Migrations follow the format: `NNNNNN_descriptive_name.{up,down}.sql`
where NNNNNN is a 6-digit sequence number (e.g., 000001).

## Running Migrations

Migrations can be run using the dbmigrate tool:

```bash
go run cmd/tools/dbmigrate/main.go -db ./data/pickleicious.db -migrations ./internal/db/migrations -command up

# Database Management

## Overview
This directory contains database migrations and related tooling for the Pickleicious application. We use golang-migrate for schema migrations and sqlc for type-safe query generation.

## Directory Structure

```
/internal/db/
├── migrations/          # Database migrations
├── queries/            # SQL queries for sqlc
│   ├── courts.sql
│   ├── schedules.sql
│   └── users.sql
├── schema/             # Master schema files
└── testdata/          # Test data and seeds
```
## Migrations

### Naming Convention
Migrations follow the format:
```
NNNNNN_descriptive_name.{up,down}.sql
```
- NNNNNN: 6-digit sequence number (e.g., 000001)
- descriptive_name: Brief description using underscores
- .up.sql: Forward migration
- .down.sql: Rollback migration

### Creating New Migrations
Create new migrations using the migrate tool:
```bash
go run cmd/tools/dbmigrate/main.go create -name description_of_change

## Running Migrations
```
# Apply all pending migrations
go run cmd/tools/dbmigrate/main.go -db ./data/pickleicious.db -migrations ./internal/db/migrations -command up

# Rollback last migration
go run cmd/tools/dbmigrate/main.go -db ./data/pickleicious.db -migrations ./internal/db/migrations -command down

# Check current version
go run cmd/tools/dbmigrate/main.go -db ./data/pickleicious.db -migrations ./internal/db/migrations -command version
```
## Best Practices
### Migration Guidelines

- Migrations must be reversible whenever possible
- Large data migrations should be done in chunks
- Avoid modifying existing migrations - create new ones instead
- Include relevant indexes in the same migration as table creation
- Test both up and down migrations before committing

### Schema Changes

- Add columns as nullable or with defaults
- Use ALTER TABLE only when necessary
- Consider data migration impact
- Document breaking changes

### Data Migrations
- Handle large datasets in batches
- Include progress logging
- Provide status updates
- Consider timeouts

### Default Data
Initial migrations include default data for:

- Facility information
- Operating hours
- Court configurations

### Testing

- Use testdata/seeds/ for test data
- Each test should setup its own database state
- Clean up after tests
- Use transactions for test isolation

## Troubleshooting
### Common Issues

- Migration Version Mismatch
```
# Check current version
go run cmd/tools/dbmigrate/main.go -command version
```
- Dirty Migration State
```
# Force version (use with caution)
go run cmd/tools/dbmigrate/main.go -command force VERSION
```
- Database Locks

    - Check for long-running transactions
    - Verify application connections are closed
    - Check for hung migrations


### Recovery Steps

1. Backup database
2. Check migration logs
3. Fix issue in a new migration
4. Test recovery procedure
5. Document incident and solution

## Future Considerations
### Turso Migration
The schema is designed to be compatible with both SQLite and Turso. When migrating to Turso:

- Update connection strings
- Verify all queries work with Turso
- Test migration procedure
- Update documentation

### Performance

- Monitor migration duration
- Index impact analysis
- Query performance tracking
- Document optimization strategies

### Tools

- golang-migrate: Schema management
- sqlc: Type-safe query generation
- sqlite3: Local development database
- Turso: Future cloud database

