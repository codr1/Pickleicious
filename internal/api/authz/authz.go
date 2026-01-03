package authz

import (
	"context"
	"errors"
)

var (
	ErrUnauthenticated = errors.New("unauthenticated")
	ErrForbidden       = errors.New("forbidden")
)

type AuthUser struct {
	ID             int64
	IsStaff        bool
	HomeFacilityID *int64
}

type userContextKey struct{}

func ContextWithUser(ctx context.Context, user *AuthUser) context.Context {
	return context.WithValue(ctx, userContextKey{}, user)
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
