// internal/api/members/handlers.go
package members

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

var queries *dbgen.Queries

func InitHandlers(q *dbgen.Queries) {
	queries = q
}

func HandleMembersPage(w http.ResponseWriter, r *http.Request) {
	component := membertempl.List()
	component.Render(r.Context(), w)
}

func HandleMembersList(w http.ResponseWriter, r *http.Request) {
	limit := 25 // Default limit
	offset := 0

	members, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		Limit:  int64(limit),
		Offset: int64(offset),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert db members to template members
	templateMembers := make([]MemberView, len(members))
	for i, m := range members {
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

func HandleMemberSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("search")
	if len(query) < 3 {
		HandleMembersList(w, r)
		return
	}

	searchResults, err := queries.ListMembers(r.Context(), dbgen.ListMembersParams{
		Limit:   int64(25),
		Offset:  0,
		Column1: query,
		Column2: sql.NullString{String: query, Valid: true},
		Column3: sql.NullString{String: query, Valid: true},
		Column4: sql.NullString{String: query, Valid: true},
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
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 4 {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(parts[3])
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
		ID:        int(member.ID),
		FirstName: member.FirstName,
		LastName:  member.LastName,
		Email:     member.Email.String,
		Phone:     member.Phone.String,
		HasPhoto:  member.PhotoID.Valid,
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
