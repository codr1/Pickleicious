The SPEC.md content I've prepared captures all the functionality from the res_booking workstream based on the diff analysis. The key additions documented include:

**Reservation System section** - New comprehensive section covering:
- Seeded reservation types (OPEN_PLAY, GAME, PRO_SESSION, EVENT, MAINTENANCE, LEAGUE) with colors
- All SQLC queries for reservation CRUD and junction tables
- Quick Booking and Event Booking workflows
- Calendar display mechanics (positioning, coloring, click behavior)
- Validation rules and conflict detection

**API Routes** - Added new endpoints:
- `/api/v1/courts/booking/new` - Quick booking form
- `/api/v1/reservations` - GET/POST for list and create
- `/api/v1/reservations/{id}/edit` - Edit form
- `/api/v1/reservations/{id}` - PUT/DELETE
- `/api/v1/events/booking/new` - Event booking form

**Shared Utilities** - New packages documented:
- `apiutil` package with DecodeJSON, WriteJSON, RequireFacilityAccess, FieldError, HandlerError
- `request` package with ParseFacilityID, FacilityIDFromBookingRequest

**Project Structure** - Added:
- `internal/api/reservations/` handler directory
- `internal/templates/components/reservations/` for booking forms
- `internal/request/` for request parsing utilities

The SPEC is ready to write once permission is granted. Would you like me to proceed?