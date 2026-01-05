// internal/api/staff/handlers.go
package staff

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	appdb "github.com/codr1/Pickleicious/internal/db"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	stafftempl "github.com/codr1/Pickleicious/internal/templates/components/staff"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var (
	queries *dbgen.Queries
	store   *appdb.DB
)

const staffQueryTimeout = 5 * time.Second

// InitHandlers must be called during server startup before handling requests.
func InitHandlers(database *appdb.DB) {
	if database == nil {
		return
	}
	queries = database.Queries
	store = database
}

// /staff
func HandleStaffPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRows, err := queries.ListStaff(ctx)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch initial staff list")
		http.Error(w, "Failed to load staff", http.StatusInternalServerError)
		return
	}

	var activeTheme *models.Theme
	if facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id")); ok {
		theme, err := models.GetActiveTheme(ctx, queries, facilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			activeTheme = nil
		} else {
			activeTheme = theme
		}
	}

	templateStaff := stafftempl.NewStaffList(staffRows)
	page := layouts.Base(stafftempl.StaffLayout(templateStaff), activeTheme)
	if err := page.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff layout")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

// /api/v1/staff
func HandleStaffList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	facilityID, hasFacility := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	search := strings.TrimSpace(r.URL.Query().Get("search"))

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	var (
		staffRows []dbgen.ListStaffRow
		err       error
	)

	switch {
	case hasFacility:
		rawRows, err := queries.ListStaffByFacility(ctx, sql.NullInt64{Int64: facilityID, Valid: true})
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to fetch staff list by facility")
			http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
			return
		}
		staffRows = make([]dbgen.ListStaffRow, 0, len(rawRows))
		for _, row := range rawRows {
			staffRows = append(staffRows, toListStaffRow(row))
		}
		if role != "" {
			staffRows = filterStaffRowsByRole(staffRows, role)
		}
	case role != "":
		rawRows, err := queries.ListStaffByRole(ctx, role)
		if err != nil {
			logger.Error().Err(err).Str("role", role).Msg("Failed to fetch staff list by role")
			http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
			return
		}
		staffRows = make([]dbgen.ListStaffRow, 0, len(rawRows))
		for _, row := range rawRows {
			staffRows = append(staffRows, toListStaffRow(row))
		}
	default:
		staffRows, err = queries.ListStaff(ctx)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to fetch staff list")
			http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
			return
		}
	}

	if search != "" {
		staffRows = filterStaffRowsBySearch(staffRows, search)
	}

	templateStaff := stafftempl.NewStaffList(staffRows)

	component := stafftempl.StaffList(templateStaff)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff list")
		http.Error(w, "Failed to render staff list", http.StatusInternalServerError)
		return
	}
}

// /api/v1/staff/{id}
func HandleStaffDetail(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid staff ID", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	staffRow, err := queries.GetStaffByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Staff not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("id", id).Msg("Failed to fetch staff")
		http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
		return
	}

	templStaff := stafftempl.NewStaff(toListStaffRow(staffRow))
	component := stafftempl.StaffDetail(templStaff)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff detail")
		http.Error(w, "Failed to render staff detail", http.StatusInternalServerError)
		return
	}
}

// /api/v1/staff/new
func HandleNewStaffForm(w http.ResponseWriter, r *http.Request) {
	facilities, err := loadFacilities(r.Context())
	if err != nil {
		log.Ctx(r.Context()).Error().Err(err).Msg("Failed to load facilities")
		http.Error(w, "Failed to load staff form", http.StatusInternalServerError)
		return
	}

	component := stafftempl.NewStaffForm(stafftempl.Staff{}, facilities)
	if err := component.Render(r.Context(), w); err != nil {
		log.Ctx(r.Context()).Error().Err(err).Msg("Failed to render staff form")
		http.Error(w, "Failed to render staff form", http.StatusInternalServerError)
		return
	}
}

func HandleCreateStaff(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil || store == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	if err := validateStaffInput(r); err != nil {
		logger.Error().Err(err).Msg("Invalid staff input")
		http.Error(w, "Invalid staff input", http.StatusBadRequest)
		return
	}

	role := strings.ToLower(strings.TrimSpace(r.FormValue("role")))
	if !staffRoleAllowed(role) {
		http.Error(w, "Invalid staff role", http.StatusBadRequest)
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email != "" {
		if _, err := queries.GetUserByEmail(ctx, sql.NullString{String: email, Valid: true}); err == nil {
			http.Error(w, "Email already exists", http.StatusConflict)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Msg("Failed to check staff email")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	phone := strings.TrimSpace(r.FormValue("phone"))
	if phone != "" {
		if _, err := queries.GetUserByPhone(ctx, sql.NullString{String: phone, Valid: true}); err == nil {
			http.Error(w, "Phone already exists", http.StatusConflict)
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Msg("Failed to check staff phone")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	homeFacilityID := sql.NullInt64{}
	if facilityID, ok := request.ParseFacilityID(r.FormValue("home_facility_id")); ok {
		homeFacilityID = sql.NullInt64{Int64: facilityID, Valid: true}
	}

	var userID int64
	var staffID int64
	err := store.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries
		var err error

		userID, err = qtx.CreateStaffUser(ctx, dbgen.CreateStaffUserParams{
			FirstName:        r.FormValue("first_name"),
			LastName:         r.FormValue("last_name"),
			Email:            sql.NullString{String: email, Valid: email != ""},
			Phone:            sql.NullString{String: phone, Valid: phone != ""},
			HomeFacilityID:   homeFacilityID,
			LocalAuthEnabled: r.FormValue("local_auth_enabled") == "on",
			StaffRole:        sql.NullString{String: role, Valid: role != ""},
			Status:           "active",
		})
		if err != nil {
			return staffCreateTxError{msg: "Failed to create staff user", err: err}
		}

		staffID, err = qtx.CreateStaff(ctx, dbgen.CreateStaffParams{
			UserID:         userID,
			FirstName:      r.FormValue("first_name"),
			LastName:       r.FormValue("last_name"),
			HomeFacilityID: homeFacilityID,
			Role:           role,
		})
		if err != nil {
			return staffCreateTxError{msg: "Failed to create staff", err: err}
		}
		return nil
	})
	if err != nil {
		var txErr staffCreateTxError
		if errors.As(err, &txErr) {
			if txErr.msg == "Failed to create staff" {
				logger.Error().Err(err).Int64("user_id", userID).Msg(txErr.msg)
			} else {
				logger.Error().Err(err).Msg(txErr.msg)
			}
			http.Error(w, txErr.msg, http.StatusInternalServerError)
			return
		}
		logger.Error().Err(err).Msg("Failed to create staff")
		http.Error(w, "Failed to create staff", http.StatusInternalServerError)
		return
	}

	if photoData := r.FormValue("photo_data"); photoData != "" {
		photoBytes, err := decodePhotoData(photoData)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode photo data")
			http.Error(w, "Invalid photo data", http.StatusBadRequest)
			return
		}

		if _, err := queries.UpsertPhoto(ctx, dbgen.UpsertPhotoParams{
			UserID:      userID,
			Data:        photoBytes,
			ContentType: "image/jpeg",
			Size:        int64(len(photoBytes)),
		}); err != nil {
			logger.Error().Err(err).Int64("user_id", userID).Msg("Failed to save staff photo")
			http.Error(w, "Failed to save staff photo", http.StatusInternalServerError)
			return
		}
	}

	createdStaff, err := queries.GetStaffByID(ctx, staffID)
	if err != nil {
		logger.Error().Err(err).Int64("id", staffID).Msg("Failed to fetch created staff")
		http.Error(w, "Failed to fetch created staff", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", "refreshStaffList")
	w.Header().Set("HX-Retarget", "#staff-detail")
	w.Header().Set("HX-Reswap", "innerHTML")

	templStaff := stafftempl.NewStaff(toListStaffRow(createdStaff))
	if err := stafftempl.StaffDetail(templStaff).Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff detail")
		http.Error(w, "Failed to render staff detail", http.StatusInternalServerError)
		return
	}
}

func HandleEditStaffForm(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	path := r.URL.Path
	parts := strings.Split(path, "/")

	if len(parts) < 4 {
		logger.Error().
			Str("path", r.URL.Path).
			Strs("parts", parts).
			Msg("Invalid path format")
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	idStr := parts[len(parts)-2]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		logger.Error().
			Err(err).
			Str("id_str", idStr).
			Msg("Invalid staff ID format")
		http.Error(w, "Invalid staff ID", http.StatusBadRequest)
		return
	}

	staffRow, err := queries.GetStaffByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Staff not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("id", id).Msg("Failed to fetch staff for edit")
		http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
		return
	}

	facilities, err := loadFacilities(r.Context())
	if err != nil {
		logger.Error().Err(err).Msg("Failed to load facilities")
		http.Error(w, "Failed to load staff form", http.StatusInternalServerError)
		return
	}

	templStaff := stafftempl.NewStaff(toListStaffRow(staffRow))
	component := stafftempl.EditStaffForm(templStaff, facilities)
	if err := component.Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff edit form")
		http.Error(w, "Failed to render staff form", http.StatusInternalServerError)
		return
	}
}

func HandleUpdateStaff(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil || store == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), staffQueryTimeout)
	defer cancel()

	parts := strings.Split(r.URL.Path, "/")
	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		logger.Error().Err(err).Msg("Invalid staff ID")
		http.Error(w, "Invalid staff ID", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	if err := validateStaffInput(r); err != nil {
		logger.Error().Err(err).Msg("Invalid staff input")
		http.Error(w, "Invalid staff input", http.StatusBadRequest)
		return
	}

	role := strings.ToLower(strings.TrimSpace(r.FormValue("role")))
	if !staffRoleAllowed(role) {
		http.Error(w, "Invalid staff role", http.StatusBadRequest)
		return
	}

	staffRow, err := queries.GetStaffByID(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "Staff not found", http.StatusNotFound)
			return
		}
		logger.Error().Err(err).Int64("id", id).Msg("Failed to fetch staff")
		http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
		return
	}

	homeFacilityID := sql.NullInt64{}
	if facilityID, ok := request.ParseFacilityID(r.FormValue("home_facility_id")); ok {
		homeFacilityID = sql.NullInt64{Int64: facilityID, Valid: true}
	}

	email := strings.TrimSpace(r.FormValue("email"))
	phone := strings.TrimSpace(r.FormValue("phone"))

	if email != "" {
		user, err := queries.GetUserByEmail(ctx, sql.NullString{String: email, Valid: true})
		if err == nil && user.ID != staffRow.UserID {
			http.Error(w, "Email already exists", http.StatusConflict)
			return
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Msg("Failed to check staff email")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	if phone != "" {
		user, err := queries.GetUserByPhone(ctx, sql.NullString{String: phone, Valid: true})
		if err == nil && user.ID != staffRow.UserID {
			http.Error(w, "Phone already exists", http.StatusConflict)
			return
		} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
			logger.Error().Err(err).Msg("Failed to check staff phone")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	}

	var photoBytes []byte
	if photoData := r.FormValue("photo_data"); photoData != "" {
		var err error
		photoBytes, err = decodePhotoData(photoData)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode photo data")
			http.Error(w, "Invalid photo data", http.StatusBadRequest)
			return
		}
	}

	err = store.RunInTx(ctx, func(txdb *appdb.DB) error {
		qtx := txdb.Queries

		if err := qtx.UpdateStaffUser(ctx, dbgen.UpdateStaffUserParams{
			ID:               staffRow.UserID,
			FirstName:        r.FormValue("first_name"),
			LastName:         r.FormValue("last_name"),
			Email:            sql.NullString{String: email, Valid: email != ""},
			Phone:            sql.NullString{String: phone, Valid: phone != ""},
			HomeFacilityID:   homeFacilityID,
			LocalAuthEnabled: r.FormValue("local_auth_enabled") == "on",
			StaffRole:        sql.NullString{String: role, Valid: role != ""},
		}); err != nil {
			return staffUpdateTxError{msg: "Failed to update staff user", err: err}
		}

		if err := qtx.UpdateStaff(ctx, dbgen.UpdateStaffParams{
			ID:             id,
			FirstName:      r.FormValue("first_name"),
			LastName:       r.FormValue("last_name"),
			HomeFacilityID: homeFacilityID,
			Role:           role,
		}); err != nil {
			return staffUpdateTxError{msg: "Failed to update staff record", err: err}
		}

		if len(photoBytes) > 0 {
			if _, err := qtx.UpsertPhoto(ctx, dbgen.UpsertPhotoParams{
				UserID:      staffRow.UserID,
				Data:        photoBytes,
				ContentType: "image/jpeg",
				Size:        int64(len(photoBytes)),
			}); err != nil {
				return staffUpdateTxError{msg: "Failed to save staff photo", err: err}
			}
		}
		return nil
	})
	if err != nil {
		var txErr staffUpdateTxError
		if errors.As(err, &txErr) {
			logger.Error().Err(err).Int64("user_id", staffRow.UserID).Msg(txErr.msg)
			http.Error(w, txErr.msg, http.StatusInternalServerError)
			return
		}
		logger.Error().Err(err).Msg("Failed to update staff")
		http.Error(w, "Failed to update staff", http.StatusInternalServerError)
		return
	}

	updatedStaff, err := queries.GetStaffByID(ctx, id)
	if err != nil {
		logger.Error().Err(err).Int64("id", id).Msg("Failed to fetch updated staff")
		http.Error(w, "Failed to fetch staff", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", "refreshStaffList")
	w.Header().Set("HX-Retarget", "#staff-detail")
	w.Header().Set("HX-Reswap", "innerHTML")

	templStaff := stafftempl.NewStaff(toListStaffRow(updatedStaff))
	if err := stafftempl.StaffDetail(templStaff).Render(r.Context(), w); err != nil {
		logger.Error().Err(err).Msg("Failed to render staff detail")
		http.Error(w, "Failed to render staff detail", http.StatusInternalServerError)
		return
	}
}

func filterStaffRowsByRole(rows []dbgen.ListStaffRow, role string) []dbgen.ListStaffRow {
	filtered := make([]dbgen.ListStaffRow, 0, len(rows))
	for _, row := range rows {
		if !strings.EqualFold(row.Role, role) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func validateStaffInput(r *http.Request) error {
	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		return fmt.Errorf("email is required")
	}
	if !strings.Contains(email, "@") || len(email) > 254 {
		return fmt.Errorf("invalid email format")
	}

	phone := strings.TrimSpace(r.FormValue("phone"))
	if phone == "" {
		return fmt.Errorf("phone is required")
	}
	if len(phone) < 10 || len(phone) > 20 {
		return fmt.Errorf("invalid phone format")
	}

	firstName := strings.TrimSpace(r.FormValue("first_name"))
	lastName := strings.TrimSpace(r.FormValue("last_name"))
	if firstName == "" || lastName == "" {
		return fmt.Errorf("first and last name are required")
	}

	return nil
}

func staffRoleAllowed(role string) bool {
	switch strings.ToLower(role) {
	case "admin", "manager", "desk", "pro":
		return true
	default:
		return false
	}
}

func loadFacilities(ctx context.Context) ([]dbgen.Facility, error) {
	if queries == nil {
		return nil, fmt.Errorf("database queries not initialized")
	}
	return queries.ListFacilities(ctx)
}

type staffCreateTxError struct {
	msg string
	err error
}

func (e staffCreateTxError) Error() string {
	return e.msg
}

func (e staffCreateTxError) Unwrap() error {
	return e.err
}

type staffUpdateTxError struct {
	msg string
	err error
}

func (e staffUpdateTxError) Error() string {
	return e.msg
}

func (e staffUpdateTxError) Unwrap() error {
	return e.err
}

func decodePhotoData(photoData string) ([]byte, error) {
	parts := strings.SplitN(photoData, ",", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid photo data format")
	}
	photoBytes, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode photo data: %w", err)
	}
	return photoBytes, nil
}

func filterStaffRowsBySearch(rows []dbgen.ListStaffRow, search string) []dbgen.ListStaffRow {
	search = strings.ToLower(strings.TrimSpace(search))
	if search == "" {
		return rows
	}

	filtered := make([]dbgen.ListStaffRow, 0, len(rows))
	for _, row := range rows {
		name := strings.TrimSpace(fmt.Sprintf("%s %s", row.FirstName, row.LastName))
		email := ""
		if row.Email.Valid {
			email = row.Email.String
		}
		role := strings.TrimSpace(row.Role)
		facility := ""
		if row.HomeFacilityID.Valid {
			facility = fmt.Sprintf("%d", row.HomeFacilityID.Int64)
		}

		haystack := strings.ToLower(strings.TrimSpace(strings.Join([]string{
			name,
			email,
			role,
			facility,
		}, " ")))
		if strings.Contains(haystack, search) {
			filtered = append(filtered, row)
		}
	}

	return filtered
}

// toListStaffRow converts any staff-like struct to ListStaffRow.
func toListStaffRow(staff interface{}) dbgen.ListStaffRow {
	switch s := staff.(type) {
	case dbgen.ListStaffRow:
		return s
	case dbgen.ListStaffByFacilityRow:
		return dbgen.ListStaffRow{
			ID:             s.ID,
			UserID:         s.UserID,
			FirstName:      s.FirstName,
			LastName:       s.LastName,
			HomeFacilityID: s.HomeFacilityID,
			Role:           s.Role,
			CreatedAt:      s.CreatedAt,
			UpdatedAt:      s.UpdatedAt,
			Email:          s.Email,
			Phone:          s.Phone,
		}
	case dbgen.ListStaffByRoleRow:
		return dbgen.ListStaffRow{
			ID:             s.ID,
			UserID:         s.UserID,
			FirstName:      s.FirstName,
			LastName:       s.LastName,
			HomeFacilityID: s.HomeFacilityID,
			Role:           s.Role,
			CreatedAt:      s.CreatedAt,
			UpdatedAt:      s.UpdatedAt,
			Email:          s.Email,
			Phone:          s.Phone,
		}
	case dbgen.GetStaffByIDRow:
		// PasswordHash is intentionally not copied into the list view struct.
		return dbgen.ListStaffRow{
			ID:               s.ID,
			UserID:           s.UserID,
			FirstName:        s.FirstName,
			LastName:         s.LastName,
			HomeFacilityID:   s.HomeFacilityID,
			Role:             s.Role,
			CreatedAt:        s.CreatedAt,
			UpdatedAt:        s.UpdatedAt,
			Email:            s.Email,
			Phone:            s.Phone,
			LocalAuthEnabled: s.LocalAuthEnabled,
			UserStatus:       s.UserStatus,
		}
	default:
		panic(fmt.Sprintf("unsupported staff type: %T", staff))
	}
}
