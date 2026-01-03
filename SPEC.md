I've prepared the updated SPEC.md. The key additions from the Open Play Session Enforcement Engine workstream are:

**New sections added:**
1. **Open Play Sessions** - Documents the session entity that tracks individual instances of open play
2. **Session Enforcement Engine** - Documents the gocron-based scheduler and enforcement logic
3. **Auto-Scale Toggle API** - Documents the PUT endpoint for staff to override auto-scaling
4. **Staff Notifications** - Moved from "Planned" to implemented, now documents the actual notification types and storage
5. **Audit Log** - Documents the audit trail for all automated decisions

**Updated sections:**
- **Technology Choices** - Added gocron v2 as the scheduler
- **Entity Relationships** - Added open_play_sessions, staff_notifications, and audit_log
- **Configuration** - Added open_play.enforcement_interval setting
- **Implementation Status** - Moved several features from "Not Yet Started" to "Operational Today"
- **Terminology** - Added Open Play Session and Enforcement definitions
- **Tech Stack Philosophy** - Added note about background processing with gocron

The spec now accurately reflects what the engine does: scheduled jobs evaluate sessions approaching cutoff, cancel under-subscribed sessions, auto-scale court allocations, notify staff, and audit all decisions.