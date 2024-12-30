// internal/api/members/handlers.go
package members

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	membertempl "github.com/codr1/Pickleicious/internal/templates/components/members"
)

// MemberView represents a member for template rendering
type MemberView struct {
	ID        int
	FirstName string
	LastName  string
	Email     string
	Phone     string
	HasPhoto  bool
	PhotoURL  string
	Status    string
}

// Add this struct for form handling
type CreateMemberRequest struct {
	FirstName     string `json:"first_name"`
	LastName      string `json:"last_name"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`
	StreetAddress string `json:"street_address"`
	City          string `json:"city"`
	State         string `json:"state"`
	PostalCode    string `json:"postal_code"`
	DateOfBirth   string `json:"date_of_birth"` // Format: YYYY-MM-DD
	WaiverSigned  bool   `json:"waiver_signed"`
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
			Status:    m.Status,
		}
	}

	// Render the list template with initial data
	component := membertempl.List()
	component.Render(r.Context(), w)
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

	templateMembers := make([]MemberView, len(searchResults))
	for i, m := range searchResults {
		templateMembers[i] = MemberView{
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
			PhotoURL:  m.PhotoURL,
			Status:    m.Status,
		}
	}
	component := membertempl.MembersList(templMembers)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	component := membertempl.MemberDetail(membertempl.Member{
		ID:            int(member.ID),
		FirstName:     member.FirstName,
		LastName:      member.LastName,
		Email:         member.Email.String,
		Phone:         member.Phone.String,
		HasPhoto:      member.PhotoID.Valid,
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
	component := membertempl.MemberForm()
	component.Render(r.Context(), w)
}

func HandleCreateMember(w http.ResponseWriter, r *http.Request) {
	logger := log.Ctx(r.Context())
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

	// Parse form data
	dob, err := time.Parse("2006-01-02", r.FormValue("date_of_birth"))
	if err != nil {
		log.Error().Err(err).Msg("Invalid date format")
		http.Error(w, "Invalid date format", http.StatusBadRequest)
		return
	}

	// Validate age
	age := time.Now().Sub(dob).Hours() / 24 / 365.25
	if age < 0 || age > 100 {
		http.Error(w, "Invalid age: must be between 0 and 100 years", http.StatusBadRequest)
		return
	}

	// Log the create member params
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

	log.Info().Interface("params", createParams).Msg("Creating member")

	member, err := queries.CreateMember(r.Context(), createParams)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create member")
		http.Error(w, "Failed to create member", http.StatusInternalServerError)
		return
	}

	log.Info().Interface("member", member).Msg("Member created successfully")

	// Render the member detail view
	component := membertempl.MemberDetail(membertempl.Member{
		ID:        int(member.ID),
		FirstName: member.FirstName,
		LastName:  member.LastName,
		Email:     member.Email.String,
		Phone:     member.Phone.String,
		Status:    member.Status,
	})
	component.Render(r.Context(), w)
}
