# Directives

## Migrations

When creating database migrations, use story numbers to prevent conflicts
between parallel workstreams:
- STORY-0023 -> migrations/000023_*.sql
- This ensures concurrent features don't create conflicting sequence numbers
