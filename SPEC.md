The SPEC.md has been prepared. Here's a summary of the key updates I made to document the facility_auth workstream:

**Authentication and Access section - expanded Authorization Model:**
- Documented the `authz` package location (`internal/api/authz`)
- Added `AuthUser` struct definition with `ID`, `IsStaff`, and `HomeFacilityID` fields
- Created authorization rules table showing staff vs non-staff vs unauthenticated access
- Documented `RequireFacilityAccess` function behavior and error codes
- Listed all 12 protected endpoints across themes (6) and open play (6) modules
- Noted that authorization failures are logged with facility_id and user_id

**Error Handling table:**
- Added 401 Unauthenticated and 403 Forbidden codes with descriptions

**Implementation Status:**
- Added "Facility Authorization | Complete | Staff restricted to home facility, admins have full access" to the Operational Today table

The updates accurately reflect what was built based on the code diff: a lightweight authorization helper that enforces facility-scoped access for staff users while allowing admins unrestricted access.