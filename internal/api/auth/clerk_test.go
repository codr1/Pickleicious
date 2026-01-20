package auth

import (
	"context"
	"testing"

	"github.com/clerk/clerk-sdk-go/v2"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
	"github.com/codr1/Pickleicious/internal/testutil"
)

func setupClerkTest(t *testing.T) {
	t.Helper()

	database := testutil.NewTestDB(t)

	// Save and restore global state
	prevQueries := queries
	prevClerkInit := clerkInitialized
	t.Cleanup(func() {
		queries = prevQueries
		clerkInitialized = prevClerkInit
	})

	queries = dbgen.New(database.DB)

	ctx := context.Background()

	// Create organization
	orgResult, err := database.ExecContext(ctx,
		"INSERT INTO organizations (name, slug, status) VALUES (?, ?, ?)",
		"Test Org", "test-org", "active",
	)
	if err != nil {
		t.Fatalf("insert organization: %v", err)
	}
	orgID, _ := orgResult.LastInsertId()

	// Create facility
	facilityResult, err := database.ExecContext(ctx,
		"INSERT INTO facilities (organization_id, name, slug, timezone) VALUES (?, ?, ?, ?)",
		orgID, "Test Facility", "test-facility", "UTC",
	)
	if err != nil {
		t.Fatalf("insert facility: %v", err)
	}
	facilityID, _ := facilityResult.LastInsertId()

	// Create test users with various identifiers
	// Using real US area codes for valid phone numbers
	_, err = database.ExecContext(ctx,
		`INSERT INTO users (email, phone, first_name, last_name, is_member, home_facility_id, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"member@test.com", "+12125551234", "Test", "Member", true, facilityID, "active",
	)
	if err != nil {
		t.Fatalf("insert member user: %v", err)
	}

	// Create user with only email
	_, err = database.ExecContext(ctx,
		`INSERT INTO users (email, first_name, last_name, is_member, home_facility_id, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"emailonly@test.com", "Email", "Only", true, facilityID, "active",
	)
	if err != nil {
		t.Fatalf("insert email-only user: %v", err)
	}

	// Create user with only phone
	_, err = database.ExecContext(ctx,
		`INSERT INTO users (phone, first_name, last_name, is_member, home_facility_id, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		"+13105559876", "Phone", "Only", true, facilityID, "active",
	)
	if err != nil {
		t.Fatalf("insert phone-only user: %v", err)
	}
}

func TestInitClerk(t *testing.T) {
	// Save and restore global state
	prevClerkInit := clerkInitialized
	t.Cleanup(func() {
		clerkInitialized = prevClerkInit
	})

	t.Run("empty secret key does not initialize", func(t *testing.T) {
		clerkInitialized = false
		InitClerk("")
		if clerkInitialized {
			t.Error("expected clerkInitialized to be false with empty key")
		}
	})

	t.Run("valid secret key initializes", func(t *testing.T) {
		clerkInitialized = false
		InitClerk("sk_test_xxx")
		if !clerkInitialized {
			t.Error("expected clerkInitialized to be true with valid key")
		}
	})
}

func TestFindLocalUserFromClerk(t *testing.T) {
	setupClerkTest(t)
	ctx := context.Background()

	t.Run("find by primary email", func(t *testing.T) {
		emailID := "email_123"
		clerkUser := &clerk.User{
			PrimaryEmailAddressID: &emailID,
			EmailAddresses: []*clerk.EmailAddress{
				{ID: emailID, EmailAddress: "member@test.com"},
			},
		}

		user, err := findLocalUserFromClerk(ctx, clerkUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email.String != "member@test.com" {
			t.Errorf("expected email member@test.com, got %s", user.Email.String)
		}
	})

	t.Run("find by primary phone with normalization", func(t *testing.T) {
		phoneID := "phone_123"
		clerkUser := &clerk.User{
			PrimaryPhoneNumberID: &phoneID,
			PhoneNumbers: []*clerk.PhoneNumber{
				{ID: phoneID, PhoneNumber: "(212) 555-1234"}, // Non-E.164 format, normalizes to +12125551234
			},
		}

		user, err := findLocalUserFromClerk(ctx, clerkUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Phone.String != "+12125551234" {
			t.Errorf("expected phone +12125551234, got %s", user.Phone.String)
		}
	})

	t.Run("find by secondary email when primary not found", func(t *testing.T) {
		emailID := "email_123"
		clerkUser := &clerk.User{
			PrimaryEmailAddressID: &emailID,
			EmailAddresses: []*clerk.EmailAddress{
				{ID: emailID, EmailAddress: "notfound@test.com"},
				{ID: "email_456", EmailAddress: "emailonly@test.com"},
			},
		}

		user, err := findLocalUserFromClerk(ctx, clerkUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email.String != "emailonly@test.com" {
			t.Errorf("expected email emailonly@test.com, got %s", user.Email.String)
		}
	})

	t.Run("find by secondary phone when primary not found", func(t *testing.T) {
		phoneID := "phone_123"
		clerkUser := &clerk.User{
			PrimaryPhoneNumberID: &phoneID,
			PhoneNumbers: []*clerk.PhoneNumber{
				{ID: phoneID, PhoneNumber: "+14155550000"},     // Valid but not in DB
				{ID: "phone_456", PhoneNumber: "(310) 555-9876"}, // In DB (normalizes to +13105559876)
			},
		}

		user, err := findLocalUserFromClerk(ctx, clerkUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Phone.String != "+13105559876" {
			t.Errorf("expected phone +13105559876, got %s", user.Phone.String)
		}
	})

	t.Run("user not found returns ErrNoRows", func(t *testing.T) {
		clerkUser := &clerk.User{
			EmailAddresses: []*clerk.EmailAddress{
				{ID: "email_999", EmailAddress: "doesnotexist@test.com"},
			},
		}

		_, err := findLocalUserFromClerk(ctx, clerkUser)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("empty clerk user returns ErrNoRows", func(t *testing.T) {
		clerkUser := &clerk.User{}

		_, err := findLocalUserFromClerk(ctx, clerkUser)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid phone format is skipped", func(t *testing.T) {
		phoneID := "phone_123"
		emailID := "email_456"
		clerkUser := &clerk.User{
			PrimaryPhoneNumberID: &phoneID,
			PhoneNumbers: []*clerk.PhoneNumber{
				{ID: phoneID, PhoneNumber: "invalid-phone"},
			},
			PrimaryEmailAddressID: &emailID,
			EmailAddresses: []*clerk.EmailAddress{
				{ID: emailID, EmailAddress: "member@test.com"},
			},
		}

		// Should skip invalid phone and find by email
		user, err := findLocalUserFromClerk(ctx, clerkUser)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Email.String != "member@test.com" {
			t.Errorf("expected to find user by email, got %s", user.Email.String)
		}
	})
}
