// internal/api/members/handlers.go
package members

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/codr1/Pickleicious/internal/api/authz"
	"github.com/codr1/Pickleicious/internal/cognito"
	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/models"
	"github.com/codr1/Pickleicious/internal/request"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/members"
	"github.com/codr1/Pickleicious/internal/templates/layouts"
)

var queries *dbgen.Queries
var cognitoClient *cognito.CognitoClient

const membersQueryTimeout = 5 * time.Second

// normalizePhoneInput normalizes a phone number to E.164 format.
// Returns empty string if input is empty, or error if invalid non-empty phone.
func normalizePhoneInput(phone string) (sql.NullString, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return sql.NullString{String: "", Valid: false}, nil
	}
	normalized := cognito.NormalizePhone(phone)
	if normalized == "" {
		return sql.NullString{}, fmt.Errorf("invalid phone number format")
	}
	return sql.NullString{String: normalized, Valid: true}, nil
}

func InitHandlers(q *dbgen.Queries, cc *cognito.CognitoClient) {
	queries = q
	cognitoClient = cc
}

// /members
func HandleMembersPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Fetch initial members list
	members, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{},
		Limit:      25, // Default limit
		Offset:     0,  // This should be passed in...
		SearchTerm: "", // Empty search string
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch initial members list")
		http.Error(w, "An error occurred", http.StatusInternalServerError)
		return
	}

	var activeTheme *models.Theme
	facilityID, ok := request.ParseFacilityID(r.URL.Query().Get("facility_id"))
	if ok {
		ctx, cancel := context.WithTimeout(r.Context(), membersQueryTimeout)
		defer cancel()

		theme, err := models.GetActiveTheme(ctx, queries, facilityID)
		if err != nil {
			logger.Error().Err(err).Int64("facility_id", facilityID).Msg("Failed to load active theme")
			theme = nil
		}
		activeTheme = theme
	}

	// Convert to template Members
	templateMembers := membertempl.NewMembers(members)

	// Render the layout template with members
	component := membertempl.MembersLayout(templateMembers)
	sessionType := authz.SessionTypeFromContext(r.Context())
	page := layouts.Base(component, activeTheme, sessionType)

	err = page.Render(r.Context(), w)
	if err != nil {
		log.Error().Err(err).Msg("Failed to render members layout")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleMembersList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	// Parse pagination parameters
	limit, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 25 // default limit
	}

	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil {
		offset = 0 // default offset
	}

	members, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{},
		Limit:      limit,
		Offset:     offset,
		SearchTerm: sql.NullString{String: "", Valid: false},
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch members")
		http.Error(w, "Failed to fetch members", http.StatusInternalServerError)
		return
	}

	// Convert to template Members
	templateMembers := membertempl.NewMembers(members)

	// Render the members list
	component := membertempl.MembersList(templateMembers)
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render members list")
		http.Error(w, "Failed to render members list", http.StatusInternalServerError)
		return
	}
}

func HandleMemberSearch(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
	searchTerm := r.URL.Query().Get("q")

	// Parse pagination parameters
	limit, err := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if err != nil || limit <= 0 {
		limit = 25 // default limit
	}

	offset, err := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	if err != nil {
		offset = 0 // default offset
	}

	members, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		FacilityID: sql.NullInt64{},
		Limit:      limit,
		Offset:     offset,
		SearchTerm: sql.NullString{String: searchTerm, Valid: true},
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to search members")
		http.Error(w, "Failed to search members", http.StatusInternalServerError)
		return
	}

	// Convert to template Members
	templateMembers := membertempl.NewMembers(members)

	// Render the members list
	component := membertempl.MembersList(templateMembers)
	err = component.Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render members list")
		http.Error(w, "Failed to render members list", http.StatusInternalServerError)
		return
	}
}

func HandleDeleteMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	// Extract ID from URL path
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	logger.Debug().
		Str("path", path).
		Strs("parts", parts).
		Msg("Delete request received")

	if len(parts) < 4 {
		logger.Error().Msg("Invalid path format")
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		logger.Error().Err(err).Str("id_str", parts[len(parts)-1]).Msg("Invalid member ID")
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	// Delete the member
	err = queries.DeleteMember(r.Context(), id)
	if err != nil {
		logger.Error().Err(err).Int64("id", id).Msg("Failed to delete member")
		http.Error(w, "Failed to delete member", http.StatusInternalServerError)
		return
	}

	// Return success message with HX-Trigger to refresh the list
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", "refreshMembersList")
	w.Write([]byte(`
		<div class="h-full flex items-center justify-center text-gray-500">
			<p>Member successfully deleted</p>
		</div>
	`))
}

func HandleEditMemberForm(w http.ResponseWriter, r *http.Request) {

	// Extract ID from URL path
	path := r.URL.Path
	parts := strings.Split(path, "/")

	// The path should be like /api/v1/members/{id}/edit
	// So we want the second-to-last part
	if len(parts) < 4 {
		log.Error().
			Str("path", r.URL.Path).
			Strs("parts", parts).
			Msg("Invalid path format")
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	idStr := parts[len(parts)-2]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		log.Error().
			Err(err).
			Str("id_str", idStr).
			Msg("Invalid member ID format")
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	// Get the member details
	member, err := queries.GetMemberByID(r.Context(), id)
	if err != nil {
		log.Error().Err(err).Int64("id", id).Msg("Failed to fetch member for edit")
		http.Error(w, "Failed to fetch member", http.StatusInternalServerError)
		return
	}

	// Convert to template Member
	templMember := membertempl.NewMember(toListMembersRow(member))

	// Render the edit form instead of detail view
	component := membertempl.EditMemberForm(templMember)
	component.Render(r.Context(), w)
}

func HandleUpdateMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	// Parse member ID from URL
	parts := strings.Split(r.URL.Path, "/")
	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		logger.Error().Err(err).Msg("Invalid member ID")
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	// Process form data
	err = r.ParseForm()
	if err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Debug form data
	logger.Debug().
		Str("first_name", r.FormValue("first_name")).
		Str("last_name", r.FormValue("last_name")).
		Str("photo_data_exists", fmt.Sprintf("%v", r.FormValue("photo_data") != "")).
		Int("photo_data_length", len(r.FormValue("photo_data"))).
		Msg("Form data received")

	// Normalize phone number to E.164 format
	phone, err := normalizePhoneInput(r.FormValue("phone"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// First update the member details
	err = queries.UpdateMember(r.Context(), dbgen.UpdateMemberParams{
		ID:            id,
		FirstName:     r.FormValue("first_name"),
		LastName:      r.FormValue("last_name"),
		Email:         sql.NullString{String: r.FormValue("email"), Valid: true},
		Phone:         phone,
		StreetAddress: sql.NullString{String: r.FormValue("street_address"), Valid: true},
		City:          sql.NullString{String: r.FormValue("city"), Valid: true},
		State:         sql.NullString{String: r.FormValue("state"), Valid: true},
		PostalCode:    sql.NullString{String: r.FormValue("postal_code"), Valid: true},
		Status:        r.FormValue("status"),
		DateOfBirth:   r.FormValue("date_of_birth"),
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to update member")
		http.Error(w, "Failed to update member", http.StatusInternalServerError)
		return
	}

	// Process photo if present
	if photoData := r.FormValue("photo_data"); photoData != "" {
		// Remove data URL prefix and decode
		photoBytes, err := base64.StdEncoding.DecodeString(strings.Split(photoData, ",")[1])
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode photo data")
			http.Error(w, "Invalid photo data", http.StatusBadRequest)
			return
		}

		// Save/Update the photo and get its ID
		photo, err := queries.UpsertPhoto(r.Context(), dbgen.UpsertPhotoParams{
			UserID:      id,
			Data:        photoBytes,
			ContentType: "image/jpeg",
			Size:        int64(len(photoBytes)),
		})
		if err != nil {
			logger.Error().
				Err(err).
				Int64("member_id", id).
				Msg("Failed to save photo")
			http.Error(w, "Failed to save photo", http.StatusInternalServerError)
			return
		}

		logger.Info().Int64("photo_Vid", photo.ID).Msg("Photo saved successfully")
	}

	// Get the updated member with photo info
	member, err := queries.GetMemberByID(r.Context(), id)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch updated member")
		http.Error(w, "Failed to fetch updated member", http.StatusInternalServerError)
		return
	}

	// Set headers and render response
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", "refreshMembersList")
	w.Header().Set("HX-Retarget", "#member-detail")
	w.Header().Set("HX-Reswap", "innerHTML")

	templMember := membertempl.NewMember(toListMembersRow(member))
	// Render response
	err = membertempl.MemberDetail(templMember).Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render member detail")
		http.Error(w, "Failed to render response", http.StatusInternalServerError)
		return
	}
}

func HandleMemberDetail(w http.ResponseWriter, r *http.Request) {
	// Trim any trailing slash and split
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	// Log the path parts for debugging
	log.Debug().
		Str("path", r.URL.Path).
		Strs("parts", parts).
		Msg("Parsing member detail path")

	// Extract ID from the last part
	if len(parts) == 0 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	idStr := parts[len(parts)-1]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	member, err := queries.GetMemberByID(r.Context(), id)
	if err != nil {
		logger := log.Ctx(r.Context())
		logger.Error().Err(err).Int64("id", id).Msg("Failed to fetch member")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// No conversion needed as GetMemberByID now returns ListMembersRow
	templMember := membertempl.NewMember(toListMembersRow(member))

	// Render the detail view
	component := membertempl.MemberDetail(templMember)
	component.Render(r.Context(), w)
}

func HandleMemberBilling(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	// Extract member ID from URL path
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid billing URL", http.StatusBadRequest)
		return
	}

	memberID, err := strconv.ParseInt(parts[len(parts)-2], 10, 64) // Use len-2 to skip "billing"
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	// Fetch billing info
	billing, err := queries.GetMemberBilling(r.Context(), memberID)
	if err != nil {
		if err == sql.ErrNoRows {
			// No billing info is not an error
			return
		}
		logger.Error().Err(err).Int64("member_id", memberID).Msg("Failed to fetch billing info")
		http.Error(w, "Failed to fetch billing info", http.StatusInternalServerError)
		return
	}

	// Render the billing component
	err = membertempl.BillingInfo(billing).Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render billing info")
		http.Error(w, "Failed to render billing info", http.StatusInternalServerError)
		return
	}
}

func HandleMemberVisits(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	user := authz.UserFromContext(r.Context())
	if user == nil || !user.IsStaff {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid visits URL", http.StatusBadRequest)
		return
	}

	memberID, err := strconv.ParseInt(parts[len(parts)-2], 10, 64)
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	visits, err := queries.ListRecentVisitsByUser(r.Context(), memberID)
	if err != nil {
		logger.Error().Err(err).Int64("member_id", memberID).Msg("Failed to fetch visit history")
		http.Error(w, "Failed to fetch visit history", http.StatusInternalServerError)
		return
	}

	facilityNames := make(map[int64]string)
	facilities, err := queries.ListFacilities(r.Context())
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch facilities for visit history")
	} else {
		for _, facility := range facilities {
			facilityNames[facility.ID] = facility.Name
		}
	}

	templateVisits := membertempl.NewVisitHistory(visits, facilityNames)
	err = membertempl.VisitsHistory(templateVisits).Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render visit history")
		http.Error(w, "Failed to render visit history", http.StatusInternalServerError)
		return
	}
}

func HandleNewMemberForm(w http.ResponseWriter, r *http.Request) {
	component := membertempl.NewMemberForm(membertempl.Member{})
	component.Render(r.Context(), w)
}

// validateMemberInput checks for basic input validation and sanitization
func validateMemberInput(r *http.Request) error {
	email := r.FormValue("email")
	if !strings.Contains(email, "@") || len(email) > 254 {
		return fmt.Errorf("invalid email format")
	}

	// Basic phone validation
	phone := r.FormValue("phone")
	if len(phone) < 10 || len(phone) > 20 {
		return fmt.Errorf("invalid phone format")
	}

	// Validate postal code
	postalCode := r.FormValue("postal_code")
	if len(postalCode) < 5 || len(postalCode) > 10 {
		return fmt.Errorf("invalid postal code")
	}

	return nil
}

// HandleCreateMember handles new member creation
// Date handling:
// 1. Input comes as YYYY-MM-DD string from form
// 2. Parsed into time.Time for validation
// 3. Passed to SQL as time.Time
// 4. SQL converts to YYYY-MM-DD string for storage
// 5. SQLite (which is our main engine at present) does not have a DATE type, so it's stored as TEXT
func HandleCreateMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if err := validateMemberInput(r); err != nil {
		log.Error().Err(err).Msg("Invalid input data")
		http.Error(w, "Invalid input data", http.StatusBadRequest)
		return
	}

	// Check if email already exists
	email := r.FormValue("email")
	existingMember, err := queries.GetMemberByEmailIncludeDeleted(r.Context(), sql.NullString{String: email, Valid: true})
	if err == nil { // Member found with this email
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict) // 409 Conflict
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":     "duplicate_email",
			"member_id": existingMember.ID,
		})
		return
	} else if err != sql.ErrNoRows {
		logger.Error().Err(err).Msg("Database error checking email")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Parse date of birth
	dobStr := r.FormValue("date_of_birth")
	dob, err := time.Parse("2006-01-02", dobStr)
	if err != nil {
		logger.Error().Err(err).Str("dob", dobStr).Msg("Invalid date format")
		http.Error(w, "Invalid date format: must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	// Validate age
	age := time.Since(dob).Hours() / 24 / 365.25
	if age < 0 || age > 100 {
		http.Error(w, "Invalid age: must be between 0 and 100 years", http.StatusBadRequest)
		return
	}

	// Normalize phone number to E.164 format
	phone, err := normalizePhoneInput(r.FormValue("phone"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Create member and get ID
	memberID, err := queries.CreateMember(r.Context(), dbgen.CreateMemberParams{
		FirstName:     r.FormValue("first_name"),
		LastName:      r.FormValue("last_name"),
		Email:         sql.NullString{String: r.FormValue("email"), Valid: true},
		Phone:         phone,
		StreetAddress: sql.NullString{String: r.FormValue("street_address"), Valid: true},
		City:          sql.NullString{String: r.FormValue("city"), Valid: true},
		State:         sql.NullString{String: r.FormValue("state"), Valid: true},
		PostalCode:    sql.NullString{String: r.FormValue("postal_code"), Valid: true},
		Status:        "active",
		DateOfBirth:   dob,
		WaiverSigned:  r.FormValue("waiver_signed") == "on",
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create member")
		http.Error(w, "Failed to create member", http.StatusInternalServerError)
		return
	}

	// Create user in Cognito for OTP authentication (email and/or SMS)
	// If Cognito is configured and fails, we must fail the request - otherwise member can't log in
	if cognitoClient != nil {
		phone := r.FormValue("phone")
		if err := cognitoClient.CreateUser(r.Context(), email, phone); err != nil {
			// Delete the member we just created since they can't log in without Cognito
			if delErr := queries.DeleteMember(r.Context(), memberID); delErr != nil {
				logger.Error().Err(delErr).Int64("member_id", memberID).Msg("Failed to rollback member after Cognito failure")
			}
			logger.Error().Err(err).Str("email", email).Str("phone", phone).Msg("Failed to create Cognito user")
			http.Error(w, "Failed to set up member authentication", http.StatusInternalServerError)
			return
		}
		logger.Info().Str("email", email).Str("phone", phone).Msg("Created Cognito user for member")
	}

	// Fetch the complete member record
	member, err := queries.GetMemberByID(r.Context(), memberID)
	if err != nil {
		logger.Error().Err(err).Int64("id", memberID).Msg("Failed to fetch created member")
		http.Error(w, "Failed to fetch created member", http.StatusInternalServerError)
		return
	}

	// Process photo if present
	if photoData := r.FormValue("photo_data"); photoData != "" {
		// Remove data URL prefix
		photoBytes, err := base64.StdEncoding.DecodeString(strings.Split(photoData, ",")[1])
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode photo data")
			http.Error(w, "Invalid photo data", http.StatusBadRequest)
			return
		}

		// Store photo in database
		photo, err := queries.UpsertPhoto(r.Context(), dbgen.UpsertPhotoParams{
			UserID:      member.ID,
			Data:        photoBytes,
			ContentType: "image/jpeg",
			Size:        int64(len(photoBytes)),
		})
		if err != nil {
			logger.Error().
				Err(err).
				Int64("member_id", memberID).
				Msg("Failed to save photo")
			http.Error(w, "Failed to save photo", http.StatusInternalServerError)
			return
		}

		logger.Info().Int64("photo_Vid", photo.ID).Msg("Photo saved successfully")
	}

	// Get the updated member with photo info
	memberResult, err := queries.GetMemberByID(r.Context(), member.ID)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch updated member")
		http.Error(w, "Failed to fetch updated member", http.StatusInternalServerError)
		return
	}

	// Set headers and render response
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", "refreshMembersList")
	w.Header().Set("HX-Retarget", "#member-detail")
	w.Header().Set("HX-Reswap", "innerHTML")

	templMember := membertempl.NewMember(toListMembersRow(memberResult))
	// Render response
	err = membertempl.MemberDetail(templMember).Render(r.Context(), w)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to render member detail")
		http.Error(w, "Failed to render response", http.StatusInternalServerError)
		return
	}
}

func HandleMemberPhoto(w http.ResponseWriter, r *http.Request) {
	// Extract member ID from URL path: /api/v1/members/photo/{id}
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 { // ["", "api", "v1", "members", "photo", "{id}"]
		http.Error(w, "Invalid photo URL", http.StatusBadRequest)
		return
	}

	memberID, err := strconv.ParseInt(parts[len(parts)-1], 10, 64)
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	// Fetch photo from database
	photo, err := queries.GetMemberPhoto(r.Context(), memberID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "Photo not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch photo", http.StatusInternalServerError)
		return
	}

	// Set content type and serve photo
	w.Header().Set("Content-Type", photo.ContentType)
	w.Write(photo.Data)
}

func HandleRestoreDecision(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	restore := r.FormValue("restore") == "true"
	oldID, _ := strconv.ParseInt(r.FormValue("old_id"), 10, 64)

	if restore {
		// First restore the member
		err := queries.RestoreMember(r.Context(), oldID)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to restore member")
			http.Error(w, "Failed to restore member", http.StatusInternalServerError)
			return
		}

		// Then fetch the restored member
		member, err := queries.GetRestoredMember(r.Context(), oldID)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to fetch restored member")
			http.Error(w, "Failed to fetch restored member", http.StatusInternalServerError)
			return
		}

		// Set HTMX headers for UI updates
		w.Header().Set("HX-Trigger", "refreshMembersList")
		w.Header().Set("HX-Retarget", "#member-detail")
		w.Header().Set("HX-Reswap", "innerHTML")

		templMember := membertempl.NewMember(toListMembersRow(member))
		component := membertempl.MemberDetail(templMember)
		component.Render(r.Context(), w)
		return
	} else {
		// Rename old account email to avoid conflict, then signal UI to retry creation
		oldEmail := r.FormValue("old_email")
		newEmail := fmt.Sprintf("%d___%s", oldID, oldEmail)

		_, err := queries.UpdateMemberEmail(r.Context(), dbgen.UpdateMemberEmailParams{
			ID:    oldID,
			Email: sql.NullString{String: newEmail, Valid: true},
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to update old member email")
			http.Error(w, "Failed to update old member email", http.StatusInternalServerError)
			return
		}

		// Tell UI the conflict is resolved and it can retry creation
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]bool{"proceed": true})
	}
}

// toListMembersRow converts any member-like struct to ListMembersRow
func toListMembersRow(member interface{}) dbgen.ListMembersRow {
	switch m := member.(type) {
	case dbgen.ListMembersRow:
		return m
	case dbgen.GetMemberByIDRow:
		// Explicit field mapping since direct conversion isn't allowed
		return dbgen.ListMembersRow{
			ID:            m.ID,
			FirstName:     m.FirstName,
			LastName:      m.LastName,
			Email:         m.Email,
			Phone:         m.Phone,
			StreetAddress: m.StreetAddress,
			City:          m.City,
			State:         m.State,
			PostalCode:    m.PostalCode,
			Status:        m.Status,
			DateOfBirth:   m.DateOfBirth,
			WaiverSigned:  m.WaiverSigned,
			CreatedAt:     m.CreatedAt,
			UpdatedAt:     m.UpdatedAt,
			PhotoID:       m.PhotoID,
		}
	default:
		panic(fmt.Sprintf("unsupported member type: %T", member))
	}
}
