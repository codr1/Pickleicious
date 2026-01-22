package authz

import (
	"context"
	"errors"
	"strconv"
	"strings"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
)

type AuthUser struct {
	ID              int64
	IsStaff         bool
	SessionType     string
	HomeFacilityID  *int64
	MembershipLevel int64
}

type StaffAccess struct {
	Role           string
	HomeFacilityID *int64
}

type userContextKey struct{}
type organizationContextKey struct{}

// Organization represents the current organization from subdomain routing.
type Organization struct {
	ID   int64
	Name string
	Slug string
}

func ContextWithUser(ctx context.Context, user *AuthUser) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
}

func ContextWithOrganization(ctx context.Context, org *Organization) context.Context {
	return context.WithValue(ctx, organizationContextKey{}, org)
}

func OrganizationFromContext(ctx context.Context) *Organization {
	if ctx == nil {
		return nil
	}
	org, ok := ctx.Value(organizationContextKey{}).(*Organization)
	if !ok {
		return nil
	}
	return org
}

// OrganizationIDString returns the org ID as a string, or empty if no org in context.
// Use this for passing org ID to templates.
func OrganizationIDString(ctx context.Context) string {
	if org := OrganizationFromContext(ctx); org != nil {
		return strconv.FormatInt(org.ID, 10)
	}
	return ""
}

func UserFromContext(ctx context.Context) *AuthUser {
	if ctx == nil {
		return nil
	}

	user, ok := ctx.Value(userContextKey{}).(*AuthUser)
	if !ok {
		return nil
	}

	return user
}

func IsStaff(user *AuthUser) bool {
	return user != nil && user.IsStaff
}

func CanManageStaff(requesterStaff, targetStaff StaffAccess) bool {
	if !strings.EqualFold(requesterStaff.Role, "admin") && !strings.EqualFold(requesterStaff.Role, "manager") {
		return false
	}

	if requesterStaff.HomeFacilityID == nil {
		return true
	}

	if targetStaff.HomeFacilityID == nil {
		return false
	}

	return *requesterStaff.HomeFacilityID == *targetStaff.HomeFacilityID
}

func SessionTypeFromContext(ctx context.Context) string {
	user := UserFromContext(ctx)
	if user == nil {
		return ""
	}
	return user.SessionType
}

func RequireFacilityAccess(ctx context.Context, requestedFacilityID int64) error {
	user := UserFromContext(ctx)
	if user == nil {
		return ErrUnauthenticated
	}

	if user.IsStaff {
		if user.HomeFacilityID == nil || *user.HomeFacilityID != requestedFacilityID {
			return ErrForbidden
		}
	}

	return nil
}
