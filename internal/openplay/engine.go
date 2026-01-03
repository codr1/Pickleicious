package openplay

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	db "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/rs/zerolog/log"
)

const (
	openPlayNotificationCancelled = "cancelled"
	openPlayNotificationScaleUp   = "scale_up"
	openPlayNotificationScaleDown = "scale_down"

	openPlayAuditCancelled = "cancelled"
	openPlayAuditScaleUp   = "scale_up"
	openPlayAuditScaleDown = "scale_down"
)

type Engine struct {
	db *db.DB
}

func NewEngine(database *db.DB) (*Engine, error) {
	if database == nil {
		return nil, errors.New("open play engine requires a database")
	}
	return &Engine{db: database}, nil
}

func (e *Engine) EvaluateSessionsApproachingCutoff(ctx context.Context, facilityID int64, comparisonTime time.Time) error {
	if e == nil || e.db == nil || e.db.Queries == nil {
		return errors.New("open play engine not initialized")
	}

	if comparisonTime.IsZero() {
		comparisonTime = time.Now()
	}

	logger := log.Ctx(ctx).With().
		Str("component", "openplay_engine").
		Int64("facility_id", facilityID).
		Time("comparison_time", comparisonTime).
		Logger()
	logger.Info().Msg("Evaluating open play sessions approaching cutoff")

	err := e.db.RunInTx(ctx, func(txdb *db.DB) error {
		sessions, err := txdb.Queries.ListOpenPlaySessionsApproachingCutoff(ctx, dbgen.ListOpenPlaySessionsApproachingCutoffParams{
			FacilityID:     facilityID,
			ComparisonTime: comparisonTime,
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to list open play sessions approaching cutoff")
			return fmt.Errorf("list open play sessions approaching cutoff: %w", err)
		}

		logger.Info().Int("session_count", len(sessions)).Msg("Found open play sessions approaching cutoff")
		for _, session := range sessions {
			sessionLogger := logger.With().
				Int64("session_id", session.ID).
				Int64("open_play_rule_id", session.OpenPlayRuleID).
				Time("start_time", session.StartTime).
				Time("end_time", session.EndTime).
				Logger()

			rule, err := txdb.Queries.GetOpenPlayRule(ctx, dbgen.GetOpenPlayRuleParams{
				ID:         session.OpenPlayRuleID,
				FacilityID: session.FacilityID,
			})
			if err != nil {
				sessionLogger.Error().Err(err).Msg("Failed to load open play rule")
				return fmt.Errorf("load open play rule %d: %w", session.OpenPlayRuleID, err)
			}

			sessionLogger.Debug().
				Str("status", session.Status).
				Int64("current_court_count", session.CurrentCourtCount).
				Msg("Evaluating open play session")

			cancelled, err := e.cancelUndersubscribedSession(ctx, txdb.Queries, session, rule)
			if err != nil {
				sessionLogger.Error().Err(err).Msg("Failed to evaluate cancellation")
				return err
			}
			if cancelled {
				continue
			}

			_, err = e.scaleSessionCourts(ctx, txdb.Queries, session, rule)
			if err != nil {
				sessionLogger.Error().Err(err).Msg("Failed to scale open play courts")
				return err
			}
		}
		return nil
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to enforce open play sessions approaching cutoff")
		return err
	}

	return nil
}

func (e *Engine) CancelUndersubscribedSession(ctx context.Context, session dbgen.OpenPlaySession, rule dbgen.OpenPlayRule) (bool, error) {
	var cancelled bool
	err := e.db.RunInTx(ctx, func(txdb *db.DB) error {
		var err error
		cancelled, err = e.cancelUndersubscribedSession(ctx, txdb.Queries, session, rule)
		return err
	})
	if err != nil {
		log.Ctx(ctx).
			Error().
			Err(err).
			Int64("session_id", session.ID).
			Int64("facility_id", session.FacilityID).
			Int64("open_play_rule_id", session.OpenPlayRuleID).
			Msg("Failed to evaluate cancellation in transaction")
		return false, err
	}
	return cancelled, nil
}

func (e *Engine) ScaleSessionCourts(ctx context.Context, session dbgen.OpenPlaySession, rule dbgen.OpenPlayRule) (bool, error) {
	var scaled bool
	err := e.db.RunInTx(ctx, func(txdb *db.DB) error {
		var err error
		scaled, err = e.scaleSessionCourts(ctx, txdb.Queries, session, rule)
		return err
	})
	if err != nil {
		log.Ctx(ctx).
			Error().
			Err(err).
			Int64("session_id", session.ID).
			Int64("facility_id", session.FacilityID).
			Int64("open_play_rule_id", session.OpenPlayRuleID).
			Msg("Failed to evaluate scaling in transaction")
		return false, err
	}
	return scaled, nil
}

func (e *Engine) cancelUndersubscribedSession(ctx context.Context, queries *dbgen.Queries, session dbgen.OpenPlaySession, rule dbgen.OpenPlayRule) (bool, error) {
	logger := log.Ctx(ctx).With().
		Str("component", "openplay_engine").
		Int64("session_id", session.ID).
		Int64("facility_id", session.FacilityID).
		Int64("open_play_rule_id", session.OpenPlayRuleID).
		Logger()

	if session.Status != "scheduled" {
		logger.Debug().
			Str("status", session.Status).
			Str("decision", "skip_cancellation").
			Msg("Skipping cancellation check for non-scheduled session")
		return false, nil
	}

	reservationID, err := lookupOpenPlayReservationID(ctx, queries, session)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to find open play reservation")
		return false, err
	}

	signups, err := countReservationParticipants(ctx, queries, reservationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to count reservation participants")
		return false, err
	}

	if signups >= rule.MinParticipants {
		logger.Debug().
			Int64("signups", signups).
			Int64("min_participants", rule.MinParticipants).
			Str("decision", "retain_session").
			Msg("Open play session meets minimum participants")
		return false, nil
	}

	existingCourts, err := listReservationCourts(ctx, queries, reservationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list reservation courts")
		return false, err
	}

	if err := removeReservationCourts(ctx, queries, reservationID, existingCourts); err != nil {
		logger.Error().Err(err).Msg("Failed to release reservation courts")
		return false, err
	}

	reason := fmt.Sprintf("Only %d signups (minimum: %d)", signups, rule.MinParticipants)
	now := time.Now()
	updatedStatus, err := queries.UpdateOpenPlaySessionStatus(ctx, dbgen.UpdateOpenPlaySessionStatusParams{
		Status:             "cancelled",
		CancelledAt:        sql.NullTime{Time: now, Valid: true},
		CancellationReason: sql.NullString{String: reason, Valid: true},
		ID:                 session.ID,
		FacilityID:         session.FacilityID,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to cancel open play session")
		return false, fmt.Errorf("cancel open play session %d: %w", session.ID, err)
	}

	updatedCourts, err := queries.UpdateOpenPlaySessionCourtCount(ctx, dbgen.UpdateOpenPlaySessionCourtCountParams{
		CurrentCourtCount: 0,
		ID:                session.ID,
		FacilityID:        session.FacilityID,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to reset open play court count after cancellation")
		return false, fmt.Errorf("reset open play court count %d: %w", session.ID, err)
	}

	beforeState, err := marshalAuditState(map[string]any{
		"status":              session.Status,
		"current_court_count": session.CurrentCourtCount,
		"reserved_courts":     len(existingCourts),
		"signups":             signups,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal cancellation audit before state")
		return false, err
	}

	afterState, err := marshalAuditState(map[string]any{
		"status":              updatedStatus.Status,
		"current_court_count": updatedCourts.CurrentCourtCount,
		"reserved_courts":     0,
		"signups":             signups,
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to marshal cancellation audit state")
		return false, err
	}

	if _, err := queries.CreateOpenPlayAuditLog(ctx, dbgen.CreateOpenPlayAuditLogParams{
		SessionID:   session.ID,
		Action:      openPlayAuditCancelled,
		BeforeState: beforeState,
		AfterState:  afterState,
		Reason:      sql.NullString{String: reason, Valid: true},
	}); err != nil {
		logger.Error().Err(err).Msg("Failed to create cancellation audit log")
		return false, fmt.Errorf("create cancel audit log for session %d: %w", session.ID, err)
	}

	notification := fmt.Sprintf("%s cancelled - only %d signups (minimum: %d)", rule.Name, signups, rule.MinParticipants)
	if _, err := queries.CreateStaffNotification(ctx, dbgen.CreateStaffNotificationParams{
		FacilityID:       session.FacilityID,
		NotificationType: openPlayNotificationCancelled,
		Message:          notification,
		RelatedSessionID: sql.NullInt64{Int64: session.ID, Valid: true},
	}); err != nil {
		logger.Error().Err(err).Msg("Failed to create cancellation staff notification")
		return false, fmt.Errorf("create cancel notification for session %d: %w", session.ID, err)
	}

	logger.Info().
		Int64("signups", signups).
		Int64("min_participants", rule.MinParticipants).
		Int("released_courts", len(existingCourts)).
		Str("decision", "cancelled").
		Msg("Cancelled open play session")

	return true, nil
}

func (e *Engine) scaleSessionCourts(ctx context.Context, queries *dbgen.Queries, session dbgen.OpenPlaySession, rule dbgen.OpenPlayRule) (bool, error) {
	logger := log.Ctx(ctx).With().
		Str("component", "openplay_engine").
		Int64("session_id", session.ID).
		Int64("facility_id", session.FacilityID).
		Int64("open_play_rule_id", session.OpenPlayRuleID).
		Logger()

	if session.Status != "scheduled" {
		logger.Debug().
			Str("status", session.Status).
			Str("decision", "skip_scaling").
			Msg("Skipping scaling for non-scheduled session")
		return false, nil
	}

	autoScaleEnabled := rule.AutoScaleEnabled
	if session.AutoScaleOverride.Valid {
		autoScaleEnabled = session.AutoScaleOverride.Bool
	}
	if !autoScaleEnabled {
		logger.Debug().
			Str("decision", "auto_scale_disabled").
			Msg("Auto-scale disabled for open play session")
		return false, nil
	}

	reservationID, err := lookupOpenPlayReservationID(ctx, queries, session)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to find open play reservation")
		return false, err
	}

	signups, err := countReservationParticipants(ctx, queries, reservationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to count reservation participants")
		return false, err
	}

	desiredCourts := clampCourtCount(ceilDiv(signups, rule.MaxParticipantsPerCourt), rule.MinCourts, rule.MaxCourts)

	existingCourts, err := listReservationCourts(ctx, queries, reservationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list reservation courts")
		return false, err
	}

	existingCount := int64(len(existingCourts))
	if desiredCourts == existingCount {
		logger.Debug().
			Int64("signups", signups).
			Int64("desired_courts", desiredCourts).
			Int64("existing_courts", existingCount).
			Str("decision", "no_scale_needed").
			Msg("Open play session already at desired court count")
		return false, nil
	}

	availableCourts, err := listAvailableCourts(ctx, queries, session, reservationID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to list available courts for scaling")
		return false, err
	}

	availabilityLimit := existingCount + int64(len(availableCourts))
	targetCourts := desiredCourts
	availabilityLimited := false
	if desiredCourts > availabilityLimit {
		targetCourts = availabilityLimit
		availabilityLimited = true
	}

	scaleReason := fmt.Sprintf("%d participants, %d per court", signups, rule.MaxParticipantsPerCourt)
	if availabilityLimited {
		scaleReason = fmt.Sprintf("%s; availability capped at %d courts", scaleReason, availabilityLimit)
	}

	if targetCourts == existingCount && availabilityLimited {
		if err := createScaleAudit(ctx, queries, session, existingCourts, existingCourts, scaleReason, openPlayAuditScaleUp); err != nil {
			return false, err
		}
		if _, err := queries.CreateStaffNotification(ctx, dbgen.CreateStaffNotificationParams{
			FacilityID:       session.FacilityID,
			NotificationType: openPlayNotificationScaleUp,
			Message:          fmt.Sprintf("%s capped at %d courts - %d participants", rule.Name, existingCount, signups),
			RelatedSessionID: sql.NullInt64{Int64: session.ID, Valid: true},
		}); err != nil {
			return false, fmt.Errorf("create capped scaling notification for session %d: %w", session.ID, err)
		}
		logger.Info().
			Int64("signups", signups).
			Int64("desired_courts", desiredCourts).
			Int64("available_courts", int64(len(availableCourts))).
			Int64("current_courts", existingCount).
			Str("decision", "availability_capped").
			Msg("Open play session scaling capped by availability")
		return true, nil
	}

	var updatedCourts []courtAssignment
	scaleAction := openPlayAuditScaleUp
	notificationType := openPlayNotificationScaleUp
	switch {
	case targetCourts > existingCount:
		needed := int(targetCourts - existingCount)
		if needed > len(availableCourts) {
			needed = len(availableCourts)
		}
		if err := addReservationCourts(ctx, queries, reservationID, availableCourts[:needed]); err != nil {
			return false, err
		}
		updatedCourts = append(existingCourts, availableCourts[:needed]...)

	case targetCourts < existingCount:
		scaleAction = openPlayAuditScaleDown
		notificationType = openPlayNotificationScaleDown
		removeCount := int(existingCount - targetCourts)
		if err := removeReservationCourts(ctx, queries, reservationID, existingCourts[len(existingCourts)-removeCount:]); err != nil {
			return false, err
		}
		updatedCourts = existingCourts[:len(existingCourts)-removeCount]
	}

	updatedSession, err := queries.UpdateOpenPlaySessionCourtCount(ctx, dbgen.UpdateOpenPlaySessionCourtCountParams{
		CurrentCourtCount: int64(len(updatedCourts)),
		ID:                session.ID,
		FacilityID:        session.FacilityID,
	})
	if err != nil {
		return false, fmt.Errorf("update open play court count %d: %w", session.ID, err)
	}

	if err := createScaleAudit(ctx, queries, session, existingCourts, updatedCourts, scaleReason, scaleAction); err != nil {
		return false, err
	}

	notification := fmt.Sprintf("%s scaled from %d to %d courts - %d participants", rule.Name, existingCount, updatedSession.CurrentCourtCount, signups)
	if _, err := queries.CreateStaffNotification(ctx, dbgen.CreateStaffNotificationParams{
		FacilityID:       session.FacilityID,
		NotificationType: notificationType,
		Message:          notification,
		RelatedSessionID: sql.NullInt64{Int64: session.ID, Valid: true},
	}); err != nil {
		return false, fmt.Errorf("create scaling notification for session %d: %w", session.ID, err)
	}

	logger.Info().
		Int64("signups", signups).
		Int64("desired_courts", desiredCourts).
		Int64("current_courts", existingCount).
		Int64("target_courts", updatedSession.CurrentCourtCount).
		Str("scale_action", scaleAction).
		Str("decision", "scaled").
		Msg("Scaled open play session courts")

	return true, nil
}

type courtAssignment struct {
	CourtID     int64
	CourtNumber int64
}

func lookupOpenPlayReservationID(ctx context.Context, queries *dbgen.Queries, session dbgen.OpenPlaySession) (int64, error) {
	reservationID, err := queries.GetOpenPlayReservationID(ctx, dbgen.GetOpenPlayReservationIDParams{
		FacilityID:     session.FacilityID,
		OpenPlayRuleID: sql.NullInt64{Int64: session.OpenPlayRuleID, Valid: true},
		StartTime:      session.StartTime,
		EndTime:        session.EndTime,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("open play reservation not found for session %d", session.ID)
		}
		return 0, fmt.Errorf("lookup reservation for session %d: %w", session.ID, err)
	}
	return reservationID, nil
}

func countReservationParticipants(ctx context.Context, queries *dbgen.Queries, reservationID int64) (int64, error) {
	count, err := queries.CountReservationParticipants(ctx, reservationID)
	if err != nil {
		return 0, fmt.Errorf("count reservation participants %d: %w", reservationID, err)
	}
	return count, nil
}

func listReservationCourts(ctx context.Context, queries *dbgen.Queries, reservationID int64) ([]courtAssignment, error) {
	rows, err := queries.ListReservationCourts(ctx, reservationID)
	if err != nil {
		return nil, fmt.Errorf("list reservation courts %d: %w", reservationID, err)
	}
	return mapReservationCourts(rows), nil
}

func listAvailableCourts(ctx context.Context, queries *dbgen.Queries, session dbgen.OpenPlaySession, reservationID int64) ([]courtAssignment, error) {
	rows, err := queries.ListAvailableCourtsForOpenPlay(ctx, dbgen.ListAvailableCourtsForOpenPlayParams{
		FacilityID:    session.FacilityID,
		ReservationID: reservationID,
		EndTime:       session.EndTime,
		StartTime:     session.StartTime,
	})
	if err != nil {
		return nil, fmt.Errorf("list available courts for session %d: %w", session.ID, err)
	}
	return mapAvailableCourts(rows), nil
}

func addReservationCourts(ctx context.Context, queries *dbgen.Queries, reservationID int64, courts []courtAssignment) error {
	for _, court := range courts {
		if err := queries.AddReservationCourt(ctx, dbgen.AddReservationCourtParams{
			ReservationID: reservationID,
			CourtID:       court.CourtID,
		}); err != nil {
			return fmt.Errorf("add reservation court %d: %w", reservationID, err)
		}
	}
	return nil
}

func removeReservationCourts(ctx context.Context, queries *dbgen.Queries, reservationID int64, courts []courtAssignment) error {
	for _, court := range courts {
		if err := queries.RemoveReservationCourt(ctx, dbgen.RemoveReservationCourtParams{
			ReservationID: reservationID,
			CourtID:       court.CourtID,
		}); err != nil {
			return fmt.Errorf("remove reservation court %d: %w", reservationID, err)
		}
	}
	return nil
}

func mapReservationCourts(rows []dbgen.ListReservationCourtsRow) []courtAssignment {
	assignments := make([]courtAssignment, 0, len(rows))
	for _, row := range rows {
		assignments = append(assignments, courtAssignment{
			CourtID:     row.CourtID,
			CourtNumber: row.CourtNumber,
		})
	}
	return assignments
}

func mapAvailableCourts(rows []dbgen.ListAvailableCourtsForOpenPlayRow) []courtAssignment {
	assignments := make([]courtAssignment, 0, len(rows))
	for _, row := range rows {
		assignments = append(assignments, courtAssignment{
			CourtID:     row.ID,
			CourtNumber: row.CourtNumber,
		})
	}
	return assignments
}

func createScaleAudit(ctx context.Context, queries *dbgen.Queries, session dbgen.OpenPlaySession, beforeCourts, afterCourts []courtAssignment, reason, action string) error {
	beforeState, err := marshalAuditState(map[string]any{
		"current_court_count": len(beforeCourts),
		"reserved_courts":     len(beforeCourts),
	})
	if err != nil {
		return err
	}

	afterState, err := marshalAuditState(map[string]any{
		"current_court_count": len(afterCourts),
		"reserved_courts":     len(afterCourts),
	})
	if err != nil {
		return err
	}

	if _, err := queries.CreateOpenPlayAuditLog(ctx, dbgen.CreateOpenPlayAuditLogParams{
		SessionID:   session.ID,
		Action:      action,
		BeforeState: beforeState,
		AfterState:  afterState,
		Reason:      sql.NullString{String: reason, Valid: true},
	}); err != nil {
		return fmt.Errorf("create scaling audit log for session %d: %w", session.ID, err)
	}
	return nil
}

func marshalAuditState(state map[string]any) (sql.NullString, error) {
	if len(state) == 0 {
		return sql.NullString{}, nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return sql.NullString{}, fmt.Errorf("marshal audit state: %w", err)
	}
	return sql.NullString{String: string(data), Valid: true}, nil
}

func ceilDiv(value, divisor int64) int64 {
	if divisor <= 0 {
		return 0
	}
	if value <= 0 {
		return 0
	}
	return (value + divisor - 1) / divisor
}

func clampCourtCount(value, minValue, maxValue int64) int64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
