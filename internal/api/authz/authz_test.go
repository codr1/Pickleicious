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

func TestCanManageStaffCorporateAdmin(t *testing.T) {
	requester := StaffAccess{
		Role:           "admin",
		HomeFacilityID: nil,
	}
	target := StaffAccess{
		Role:           "desk",
		HomeFacilityID: nil,
	}

	if !CanManageStaff(requester, target) {
		t.Fatalf("expected corporate admin to manage staff")
	}
}

func TestCanManageStaffFacilityMatch(t *testing.T) {
	facilityID := int64(1)
	requester := StaffAccess{
		Role:           "manager",
		HomeFacilityID: &facilityID,
	}
	target := StaffAccess{
		Role:           "pro",
		HomeFacilityID: &facilityID,
	}

	if !CanManageStaff(requester, target) {
		t.Fatalf("expected facility manager to manage staff at same facility")
	}
}

func TestCanManageStaffFacilityMismatch(t *testing.T) {
	requesterFacilityID := int64(1)
	targetFacilityID := int64(2)
	requester := StaffAccess{
		Role:           "manager",
		HomeFacilityID: &requesterFacilityID,
	}
	target := StaffAccess{
		Role:           "pro",
		HomeFacilityID: &targetFacilityID,
	}

	if CanManageStaff(requester, target) {
		t.Fatalf("expected facility manager to be denied for other facility")
	}
}

func TestCanManageStaffFacilityTargetCorporate(t *testing.T) {
	facilityID := int64(1)
	requester := StaffAccess{
		Role:           "manager",
		HomeFacilityID: &facilityID,
	}
	target := StaffAccess{
		Role:           "admin",
		HomeFacilityID: nil,
	}

	if CanManageStaff(requester, target) {
		t.Fatalf("expected facility manager to be denied for corporate staff")
	}
}

func TestCanManageStaffInsufficientRole(t *testing.T) {
	facilityID := int64(1)
	requester := StaffAccess{
		Role:           "desk",
		HomeFacilityID: &facilityID,
	}
	target := StaffAccess{
		Role:           "pro",
		HomeFacilityID: &facilityID,
	}

	if CanManageStaff(requester, target) {
		t.Fatalf("expected non-admin role to be denied")
	}
}
