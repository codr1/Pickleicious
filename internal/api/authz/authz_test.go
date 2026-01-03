package authz

import (
	"context"
	"errors"
	"testing"
)

func TestRequireFacilityAccessUnauthenticated(t *testing.T) {
	err := RequireFacilityAccess(context.Background(), 1)
	if !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected ErrUnauthenticated, got %v", err)
	}
}

func TestRequireFacilityAccessStaffForbidden(t *testing.T) {
	homeFacilityID := int64(2)
	ctx := ContextWithUser(context.Background(), &AuthUser{
		ID:             10,
		IsStaff:        true,
		HomeFacilityID: &homeFacilityID,
	})

	err := RequireFacilityAccess(ctx, 1)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestRequireFacilityAccessStaffNilHomeFacilityForbidden(t *testing.T) {
	ctx := ContextWithUser(context.Background(), &AuthUser{
		ID:      10,
		IsStaff: true,
	})

	err := RequireFacilityAccess(ctx, 1)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
}

func TestRequireFacilityAccessNonStaffAllowed(t *testing.T) {
	ctx := ContextWithUser(context.Background(), &AuthUser{
		ID:      10,
		IsStaff: false,
	})

	err := RequireFacilityAccess(ctx, 1)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRequireFacilityAccessStaffAllowed(t *testing.T) {
	homeFacilityID := int64(1)
	ctx := ContextWithUser(context.Background(), &AuthUser{
		ID:             10,
		IsStaff:        true,
		HomeFacilityID: &homeFacilityID,
	})

	err := RequireFacilityAccess(ctx, 1)
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
