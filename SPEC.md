The SPEC.md has been prepared. Here is a summary of the key updates from the facility authorization and Open Play Session Enforcement Engine workstreams.

**Authentication and Access section - expanded Authorization Model:**
- Documented the `authz` package location (`internal/api/authz`)
- Added `AuthUser` struct definition with `ID`, `IsStaff`, and `HomeFacilityID` fields
- Created authorization rules table showing staff vs non-staff vs unauthenticated access
- Documented `RequireFacilityAccess` function behavior and error codes
- Listed all 12 protected endpoints across themes (6) and open play (6) modules
- Noted that authorization failures are logged with facility_id and user_id

**Error Handling table:**
- Added 401 Unauthenticated and 403 Forbidden codes with descriptions

**Open Play Session Enforcement Engine - new sections added:**
1. **Open Play Sessions** - Documents the session entity that tracks individual instances of open play
2. **Session Enforcement Engine** - Documents the gocron-based scheduler and enforcement logic
3. **Auto-Scale Toggle API** - Documents the PUT endpoint for staff to override auto-scaling
4. **Staff Notifications** - Moved from "Planned" to implemented, now documents the actual notification types and storage
5. **Audit Log** - Documents the audit trail for all automated decisions

**Updated sections:**
- **Technology Choices** - Added gocron v2 as the scheduler
- **Entity Relationships** - Added open_play_sessions, staff_notifications, and audit_log
- **Configuration** - Added open_play.enforcement_interval setting
- **Implementation Status** - Added facility authorization to "Operational Today" and moved enforcement features from "Not Yet Started" to "Operational Today"
- **Terminology** - Added Open Play Session and Enforcement definitions
- **Tech Stack Philosophy** - Added note about background processing with gocron

The updates accurately reflect what was built: a lightweight authorization helper that enforces facility-scoped access for staff users while allowing admins unrestricted access, plus the scheduled enforcement engine that evaluates sessions, auto-scales courts, notifies staff, and audits decisions.
