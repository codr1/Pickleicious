// internal/api/members/handlers.go
package members

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/members"
)

// Remove the CreateMemberRequest struct if you want to use Member for both
type CreateMemberRequest struct {
	FirstName     string    `json:"first_name"`
	LastName      string    `json:"last_name"`
	Email         string    `json:"email"`
	Phone         string    `json:"phone"`
	StreetAddress string    `json:"street_address"`
	City          string    `json:"city"`
	State         string    `json:"state"`
	PostalCode    string    `json:"postal_code"`
	DateOfBirth   time.Time `json:"date_of_birth"` // Format: YYYY-MM-DD
	WaiverSigned  bool      `json:"waiver_signed"`
}

var queries *dbgen.Queries

func InitHandlers(q *dbgen.Queries) {
	queries = q
}

func HandleMembersPage(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Fetch initial members list
	members, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		Limit:      25, // Default limit
		Offset:     0,
		SearchTerm: "", // Empty search string
	})
	if err != nil {
		logger.Error().Err(err).Msg("Failed to fetch initial members list")
		http.Error(w, "Failed to fetch members", http.StatusInternalServerError)
		return
	}

	// Convert database members to template members
	templMembers := make([]membertempl.Member, len(members))
	for i, m := range members {
		templMembers[i] = membertempl.Member{
			ID:        int(m.ID),
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Email:     m.Email.String,
			Phone:     m.Phone.String,
			HasPhoto:  m.PhotoID.Valid,
			PhotoUrl:  getPhotoURL(m.ID, m.PhotoID.Valid),
			Status:    m.Status,
		}
	}

	// Render the layout template
	component := membertempl.MembersLayout()
	err = component.Render(r.Context(), w)
	if err != nil {
		log.Error().Err(err).Msg("Failed to render members layout")
		http.Error(w, "Failed to render page", http.StatusInternalServerError)
		return
	}
}

func HandleMembersList(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	if queries == nil {
		logger.Error().Msg("Database queries not initialized")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	limit := 25 // Default limit
	offset := 0

	logger.Debug().
		Int("limit", limit).
		Int("offset", offset).
		Msg("Fetching members list")

	members, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		Limit:      int64(limit),
		Offset:     int64(offset),
		SearchTerm: "", // Empty search string
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert database members directly to template members
	templMembers := make([]membertempl.Member, len(members))
	for i, m := range members {
		templMembers[i] = membertempl.Member{
			ID:        int(m.ID),
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Email:     m.Email.String,
			Phone:     m.Phone.String,
			HasPhoto:  m.PhotoID.Valid,
			PhotoUrl:  getPhotoURL(m.ID, m.PhotoID.Valid),
			Status:    m.Status,
		}
	}

	component := membertempl.MembersList(templMembers)
	component.Render(r.Context(), w)
}

func HandleMemberSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("search")
	if len(query) < 3 {
		HandleMembersList(w, r)
		return
	}

	searchResults, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		Limit:      int64(25),
		Offset:     0,
		SearchTerm: query,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	templateMembers := make([]membertempl.Member, len(searchResults))
	for i, m := range searchResults {
		templateMembers[i] = membertempl.Member{
			ID:        int(m.ID),
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Email:     m.Email.String,
			Phone:     m.Phone.String,
			HasPhoto:  m.PhotoID.Valid,
		}
	}

	// Convert MemberView to members.Member
	templMembers := make([]membertempl.Member, len(templateMembers))
	for i, m := range templateMembers {
		templMembers[i] = membertempl.Member{
			ID:        m.ID,
			FirstName: m.FirstName,
			LastName:  m.LastName,
			Email:     m.Email,
			Phone:     m.Phone,
			HasPhoto:  m.HasPhoto,
			PhotoUrl:  m.PhotoUrl,
			Status:    m.Status,
		}
	}
	component := membertempl.MembersList(templMembers)
	component.Render(r.Context(), w)
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
	//log.Info().Str("original_url", r.URL.String()).Str("path", r.URL.Path).Msg("Edit member form request received")

	// Extract ID from URL path
	path := r.URL.Path
	parts := strings.Split(path, "/")

	//log.Info().Str("path", path).Strs("parts", parts).Msg("URL parts")

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

	// Convert to template Member struct
	templMember := membertempl.Member{
		ID:            int(member.ID),
		FirstName:     member.FirstName,
		LastName:      member.LastName,
		Email:         member.Email.String,
		Phone:         member.Phone.String,
		StreetAddress: member.StreetAddress.String,
		City:          member.City.String,
		State:         member.State.String,
		PostalCode:    member.PostalCode.String,
		DateOfBirth:   member.DateOfBirth,
		WaiverSigned:  member.WaiverSigned,
		Status:        member.Status,
	}

	// Render the edit form
	component := membertempl.EditMemberForm(templMember)
	component.Render(r.Context(), w)
}

func HandleUpdateMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	// Parse form data
	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Extract ID from URL path
	path := strings.TrimSuffix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
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

	// Parse date of birth
	dobStr := r.FormValue("date_of_birth")
	dob, err := time.Parse("2006-01-02", dobStr)
	if err != nil {
		logger.Error().
			Err(err).
			Str("dob_input", dobStr).
			Msg("Failed to parse date of birth")
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

	// Create update params
	updateParams := dbgen.UpdateMemberParams{
		ID:            id,
		FirstName:     r.FormValue("first_name"),
		LastName:      r.FormValue("last_name"),
		Email:         sql.NullString{String: r.FormValue("email"), Valid: true},
		Phone:         sql.NullString{String: r.FormValue("phone"), Valid: true},
		StreetAddress: sql.NullString{String: r.FormValue("street_address"), Valid: true},
		City:          sql.NullString{String: r.FormValue("city"), Valid: true},
		State:         sql.NullString{String: r.FormValue("state"), Valid: true},
		PostalCode:    sql.NullString{String: r.FormValue("postal_code"), Valid: true},
		Status:        "active",
		DateOfBirth:   dob,
		WaiverSigned:  r.FormValue("waiver_signed") == "on",
	}

	// Update the member
	member, err := queries.UpdateMember(r.Context(), updateParams)
	if err != nil {
		logger.Error().Err(err).Interface("params", updateParams).Msg("Failed to update member")
		http.Error(w, "Failed to update member", http.StatusInternalServerError)
		return
	}

	// Process photo if present
	photoData := r.FormValue("photo_data")
	if photoData != "" {
		// Remove data URL prefix
		photoBytes := photoData[strings.IndexByte(photoData, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(photoBytes)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode photo data")
			http.Error(w, "Invalid photo data", http.StatusBadRequest)
			return
		}

		// Store photo in database
		_, err = queries.SaveMemberPhoto(r.Context(), dbgen.SaveMemberPhotoParams{
			MemberID:    member.ID,
			Data:        decoded,
			ContentType: "image/jpeg",
			Size:        int64(len(decoded)),
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to store photo")
			http.Error(w, "Failed to store photo", http.StatusInternalServerError)
			return
		}

		// Update member's photo ID in the members table
		err = queries.UpdateMemberPhotoID(r.Context(), dbgen.UpdateMemberPhotoIDParams{
			ID:       member.ID,
			PhotoUrl: sql.NullString{String: getPhotoURL(member.ID, true), Valid: true},
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to update member's photo ID")
			http.Error(w, "Failed to update member's photo ID", http.StatusInternalServerError)
			return
		}

		// Update member's photo URL
		member.PhotoUrl = sql.NullString{String: getPhotoURL(member.ID, true), Valid: true}
	}

	// Convert to template Member struct and render detail view
	templMember := membertempl.Member{
		ID:            int(member.ID),
		FirstName:     member.FirstName,
		LastName:      member.LastName,
		Email:         member.Email.String,
		Phone:         member.Phone.String,
		StreetAddress: member.StreetAddress.String,
		City:          member.City.String,
		State:         member.State.String,
		PostalCode:    member.PostalCode.String,
		DateOfBirth:   member.DateOfBirth,
		WaiverSigned:  member.WaiverSigned,
		Status:        member.Status,
		HasPhoto:      member.PhotoUrl.Valid,
		PhotoUrl:      member.PhotoUrl.String,
	}

	component := membertempl.MemberDetail(templMember)
	component.Render(r.Context(), w)
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
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	member, err := queries.GetMemberByID(r.Context(), int64(id))
	if err != nil {
		logger := log.Ctx(r.Context())
		logger.Error().Err(err).Int("id", id).Msg("Failed to fetch member")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Add debug logging
	logger := log.Ctx(r.Context())
	logger.Debug().
		Interface("db_member", member).
		Msg("Raw database member data")

	// Log each field conversion
	templMember := membertempl.Member{
		ID: int(member.ID),
	}
	logger.Debug().
		Int("template_id", templMember.ID).
		Int64("db_id", member.ID).
		Msg("Converting member ID")

	// Generate photo URL if member has a photo
	photoURL := ""
	if member.PhotoID.Valid {
		photoURL = fmt.Sprintf("/api/v1/members/photo/%d", member.ID)
	}

	component := membertempl.MemberDetail(membertempl.Member{
		ID:            int(member.ID),
		FirstName:     member.FirstName,
		LastName:      member.LastName,
		Email:         member.Email.String,
		Phone:         member.Phone.String,
		HasPhoto:      member.PhotoID.Valid,
		PhotoUrl:      photoURL,
		Status:        member.Status,
		StreetAddress: member.StreetAddress.String,
		City:          member.City.String,
		State:         member.State.String,
		PostalCode:    member.PostalCode.String,
		DateOfBirth:   member.DateOfBirth,
		WaiverSigned:  member.WaiverSigned,
		CardLastFour:  member.CardLastFour.String,
		CardType:      member.CardType.String,
	})
	component.Render(r.Context(), w)
}

func HandleMemberBilling(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(strings.Split(r.URL.Path, "/")[3])
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	member, err := queries.GetMemberByID(r.Context(), int64(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// TODO: Create billing info template component
	billing := map[string]interface{}{
		"cardType": member.CardType.String,
		"lastFour": member.CardLastFour.String,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(billing)
}

func HandleNewMemberForm(w http.ResponseWriter, r *http.Request) {
	component := membertempl.MemberForm(membertempl.Member{})
	component.Render(r.Context(), w)
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

	email := r.FormValue("email")
	existingMember, err := queries.GetMemberByEmail(r.Context(), sql.NullString{String: email, Valid: true})

	if err == nil && existingMember.Status == "deleted" {
		// Convert db member to template member
		templMember := membertempl.Member{
			ID:        int(existingMember.ID),
			FirstName: existingMember.FirstName,
			LastName:  existingMember.LastName,
			Email:     existingMember.Email.String,
			Phone:     existingMember.Phone.String,
			Status:    existingMember.Status,
			HasPhoto:  existingMember.PhotoUrl.Valid,
			PhotoUrl:  existingMember.PhotoUrl.String,
		}
		// Found deleted member with same email
		component := membertempl.RestorePrompt(templMember, r.Form)
		component.Render(r.Context(), w)
		return
	}

	requestID := r.Context().Value("request_id").(string)

	logger.Info().
		Str("request_id", requestID).
		Str("method", r.Method).
		Str("content_type", r.Header.Get("Content-Type")).
		Msg("Member creation request received")

	if err := r.ParseForm(); err != nil {
		logger.Error().Err(err).Msg("Failed to parse form")
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Log all form values
	formData := make(map[string]string)
	for key, values := range r.Form {
		if len(values) > 0 {
			formData[key] = values[0]
		}
	}

	logger.Info().
		Str("request_id", requestID).
		Interface("form_data", formData).
		Msg("Processing member creation")

	// Log the form values
	log.Info().
		Str("request_id", requestID).
		Str("first_name", r.FormValue("first_name")).
		Str("last_name", r.FormValue("last_name")).
		Str("email", r.FormValue("email")).
		Str("date_of_birth", r.FormValue("date_of_birth")).
		Str("waiver_signed", r.FormValue("waiver_signed")).
		Msg("Received form data")

	// Parse date of birth from YYYY-MM-DD format
	dobStr := r.FormValue("date_of_birth")
	dob, err := time.Parse("2006-01-02", dobStr)
	if err != nil {
		log.Error().Err(err).Str("dob", dobStr).Msg("Invalid date format")
		http.Error(w, "Invalid date format: must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	// Validate age
	age := time.Since(dob).Hours() / 24 / 365.25
	if age < 0 || age > 100 {
		http.Error(w, "Invalid age: must be between 0 and 100 years", http.StatusBadRequest)
		return
	}

	// Create member first
	createParams := dbgen.CreateMemberParams{
		FirstName:     r.FormValue("first_name"),
		LastName:      r.FormValue("last_name"),
		Email:         sql.NullString{String: r.FormValue("email"), Valid: true},
		Phone:         sql.NullString{String: r.FormValue("phone"), Valid: true},
		StreetAddress: sql.NullString{String: r.FormValue("street_address"), Valid: true},
		City:          sql.NullString{String: r.FormValue("city"), Valid: true},
		State:         sql.NullString{String: r.FormValue("state"), Valid: true},
		PostalCode:    sql.NullString{String: r.FormValue("postal_code"), Valid: true},
		Status:        "active",
		DateOfBirth:   dob,
		WaiverSigned:  r.FormValue("waiver_signed") == "on",
	}

	member, err := queries.CreateMember(r.Context(), createParams)
	if err != nil {
		logger.Error().Err(err).Msg("Failed to create member")
		http.Error(w, "Failed to create member", http.StatusInternalServerError)
		return
	}

	// Then process photo if present
	photoData := r.FormValue("photo_data")
	if photoData != "" {
		// Remove data URL prefix
		photoBytes := photoData[strings.IndexByte(photoData, ',')+1:]
		decoded, err := base64.StdEncoding.DecodeString(photoBytes)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to decode photo data")
			http.Error(w, "Invalid photo data", http.StatusBadRequest)
			return
		}

		// Store photo in database
		_, err = queries.SaveMemberPhoto(r.Context(), dbgen.SaveMemberPhotoParams{
			MemberID:    member.ID,
			Data:        decoded,
			ContentType: "image/jpeg",
			Size:        int64(len(decoded)),
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to store photo")
			http.Error(w, "Failed to store photo", http.StatusInternalServerError)
			return
		}

		// Update member's photo ID in the members table
		err = queries.UpdateMemberPhotoID(r.Context(), dbgen.UpdateMemberPhotoIDParams{
			ID:       member.ID,
			PhotoUrl: sql.NullString{String: getPhotoURL(member.ID, true), Valid: true},
		})
		if err != nil {
			logger.Error().Err(err).Msg("Failed to update member's photo ID")
			http.Error(w, "Failed to update member's photo ID", http.StatusInternalServerError)
			return
		}

		// Update member's photo URL
		member.PhotoUrl = sql.NullString{String: getPhotoURL(member.ID, true), Valid: true}
	}

	log.Info().Interface("member", member).Msg("Member created successfully")

	// Convert to template Member struct
	templMember := membertempl.Member{
		ID:            int(member.ID),
		FirstName:     member.FirstName,
		LastName:      member.LastName,
		Email:         member.Email.String,
		Phone:         member.Phone.String,
		Status:        member.Status,
		HasPhoto:      member.PhotoUrl.Valid,
		PhotoUrl:      member.PhotoUrl.String,
		StreetAddress: member.StreetAddress.String,
		City:          member.City.String,
		State:         member.State.String,
		PostalCode:    member.PostalCode.String,
		DateOfBirth:   member.DateOfBirth,
		WaiverSigned:  member.WaiverSigned,
	}

	// Set headers before writing response
	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("HX-Trigger", "refreshMembersList")
	w.Header().Set("HX-Retarget", "#member-detail")
	w.Header().Set("HX-Reswap", "innerHTML")

	// Render the member detail view
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

func getPhotoURL(id int64, hasPhoto bool) string {
	if hasPhoto {
		return fmt.Sprintf("/api/v1/members/photo/%d", id)
	}
	return ""
}

func HandleRestoreDecision(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())

	restore := r.FormValue("restore") == "true"
	oldID, _ := strconv.ParseInt(r.FormValue("old_id"), 10, 64)

	if restore {
		// Restore the old account
		member, err := queries.RestoreMember(r.Context(), oldID)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to restore member")
			http.Error(w, "Failed to restore member", http.StatusInternalServerError)
			return
		}

		// Set the HX-Trigger header to refresh the members list
		w.Header().Set("HX-Trigger", "refreshMembersList")

		// Convert to template Member struct and render...
		templMember := membertempl.Member{
			ID:            int(member.ID),
			FirstName:     member.FirstName,
			LastName:      member.LastName,
			Email:         member.Email.String,
			Phone:         member.Phone.String,
			StreetAddress: member.StreetAddress.String,
			City:          member.City.String,
			State:         member.State.String,
			PostalCode:    member.PostalCode.String,
			DateOfBirth:   member.DateOfBirth,
			WaiverSigned:  member.WaiverSigned,
			Status:        member.Status,
			HasPhoto:      member.PhotoUrl.Valid,
			PhotoUrl:      member.PhotoUrl.String,
		}

		// Render the detail view
		component := membertempl.MemberDetail(templMember)
		component.Render(r.Context(), w)
		return
	} else {
		// Update old account email and create new one
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

		// Continue with creating new member...
		// [Your existing creation code]
	}
}
