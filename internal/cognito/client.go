package cognito

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/nyaruka/phonenumbers"
)

// ErrCognitoThrottled marks errors returned when Cognito throttles requests.
var ErrCognitoThrottled = errors.New("cognito throttling")

// ErrCognitoNotAuthorized marks errors returned when Cognito rejects credentials.
var ErrCognitoNotAuthorized = errors.New("cognito not authorized")

// ErrCognitoExpiredCode marks errors returned when Cognito sees expired codes.
var ErrCognitoExpiredCode = errors.New("cognito code expired")

// ErrCognitoCodeMismatch marks errors returned when Cognito sees mismatched codes.
var ErrCognitoCodeMismatch = errors.New("cognito code mismatch")

// ErrCognitoUserExists marks errors returned when trying to create an existing user.
var ErrCognitoUserExists = errors.New("cognito user already exists")

// ErrInvalidPhone is returned when a phone number cannot be normalized to E.164 format.
var ErrInvalidPhone = errors.New("invalid phone number")

type CognitoClient struct {
	client   *cognitoidentityprovider.Client
	poolID   string
	clientID string
}

// NewClient creates a new Cognito client from pool ID and client ID.
// The region is extracted from the pool ID (format: "region_poolid").
func NewClient(poolID, clientID string) (*CognitoClient, error) {
	region, err := regionFromPoolID(poolID)
	if err != nil {
		return nil, err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &CognitoClient{
		client:   cognitoidentityprovider.NewFromConfig(awsCfg),
		poolID:   poolID,
		clientID: clientID,
	}, nil
}

// InitiateOTP starts EMAIL_OTP or SMS_OTP authentication based on identifier type.
// Returns a session token to use with VerifyOTP.
func (c *CognitoClient) InitiateOTP(ctx context.Context, identifier string) (*cognitoidentityprovider.InitiateAuthOutput, error) {
	challenge := "EMAIL_OTP"
	username := identifier

	if IsPhoneNumber(identifier) {
		normalized := NormalizePhone(identifier)
		if normalized == "" {
			return nil, ErrInvalidPhone
		}
		challenge = "SMS_OTP"
		username = normalized
	}

	out, err := c.client.InitiateAuth(ctx, &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow: types.AuthFlowTypeUserAuth,
		ClientId: aws.String(c.clientID),
		AuthParameters: map[string]string{
			"USERNAME":            username,
			"PREFERRED_CHALLENGE": challenge,
		},
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

// VerifyOTP verifies the OTP code sent via email or SMS.
func (c *CognitoClient) VerifyOTP(ctx context.Context, session, identifier, code string) (*cognitoidentityprovider.RespondToAuthChallengeOutput, error) {
	challengeName := types.ChallengeNameTypeEmailOtp
	codeKey := "EMAIL_OTP_CODE"
	username := identifier

	if IsPhoneNumber(identifier) {
		normalized := NormalizePhone(identifier)
		if normalized == "" {
			return nil, ErrInvalidPhone
		}
		challengeName = types.ChallengeNameTypeSmsOtp
		codeKey = "SMS_OTP_CODE"
		username = normalized
	}

	out, err := c.client.RespondToAuthChallenge(ctx, &cognitoidentityprovider.RespondToAuthChallengeInput{
		ChallengeName: challengeName,
		ClientId:      aws.String(c.clientID),
		Session:       aws.String(session),
		ChallengeResponses: map[string]string{
			"USERNAME": username,
			codeKey:    code,
		},
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

// CreateUser creates a new user in the Cognito User Pool.
// The user is created with email_verified=true and no welcome email is sent.
// If phone is provided and valid, it's also added with phone_number_verified=true.
// Invalid phone numbers are silently ignored (user created with email only).
func (c *CognitoClient) CreateUser(ctx context.Context, email, phone string) error {
	attrs := []types.AttributeType{
		{Name: aws.String("email"), Value: aws.String(email)},
		{Name: aws.String("email_verified"), Value: aws.String("true")},
	}

	if phone != "" {
		normalized := NormalizePhone(phone)
		if normalized != "" {
			attrs = append(attrs,
				types.AttributeType{Name: aws.String("phone_number"), Value: aws.String(normalized)},
				types.AttributeType{Name: aws.String("phone_number_verified"), Value: aws.String("true")},
			)
		}
		// Invalid phone silently ignored - user can still auth via email
	}

	_, err := c.client.AdminCreateUser(ctx, &cognitoidentityprovider.AdminCreateUserInput{
		UserPoolId:     aws.String(c.poolID),
		Username:       aws.String(email),
		MessageAction:  types.MessageActionTypeSuppress, // Don't send welcome email
		UserAttributes: attrs,
	})
	if err != nil {
		return mapCognitoError(err)
	}
	return nil
}

// ForgotPassword initiates the password reset flow for staff users.
func (c *CognitoClient) ForgotPassword(ctx context.Context, username string) (*cognitoidentityprovider.ForgotPasswordOutput, error) {
	out, err := c.client.ForgotPassword(ctx, &cognitoidentityprovider.ForgotPasswordInput{
		ClientId: aws.String(c.clientID),
		Username: aws.String(username),
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

// ConfirmForgotPassword confirms the password reset with a code and new password.
func (c *CognitoClient) ConfirmForgotPassword(ctx context.Context, username, code, newPassword string) (*cognitoidentityprovider.ConfirmForgotPasswordOutput, error) {
	out, err := c.client.ConfirmForgotPassword(ctx, &cognitoidentityprovider.ConfirmForgotPasswordInput{
		ClientId:         aws.String(c.clientID),
		Username:         aws.String(username),
		ConfirmationCode: aws.String(code),
		Password:         aws.String(newPassword),
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

func mapCognitoError(err error) error {
	var throttled *types.TooManyRequestsException
	if errors.As(err, &throttled) {
		return fmt.Errorf("%w: %v", ErrCognitoThrottled, err)
	}
	var notAuthorized *types.NotAuthorizedException
	if errors.As(err, &notAuthorized) {
		return fmt.Errorf("%w: %v", ErrCognitoNotAuthorized, err)
	}
	var expired *types.ExpiredCodeException
	if errors.As(err, &expired) {
		return fmt.Errorf("%w: %v", ErrCognitoExpiredCode, err)
	}
	var mismatch *types.CodeMismatchException
	if errors.As(err, &mismatch) {
		return fmt.Errorf("%w: %v", ErrCognitoCodeMismatch, err)
	}
	var userExists *types.UsernameExistsException
	if errors.As(err, &userExists) {
		return fmt.Errorf("%w: %v", ErrCognitoUserExists, err)
	}
	return err
}

func regionFromPoolID(poolID string) (string, error) {
	parts := strings.SplitN(poolID, "_", 2)
	if len(parts) < 2 || parts[0] == "" {
		return "", fmt.Errorf("invalid cognito pool id: %q", poolID)
	}
	return parts[0], nil
}

// IsPhoneNumber returns true if the identifier looks like a phone number.
// Uses libphonenumber for proper international phone detection.
func IsPhoneNumber(identifier string) bool {
	// Emails always contain @, phones never do
	if strings.Contains(identifier, "@") {
		return false
	}
	if identifier == "" {
		return false
	}
	// Try to parse as phone number (default to US region for numbers without country code)
	num, err := phonenumbers.Parse(identifier, "US")
	if err != nil {
		return false
	}
	return phonenumbers.IsValidNumber(num)
}

// NormalizePhone converts phone to E.164 format using libphonenumber.
// Handles all international formats. Defaults to US for numbers without country code.
// Returns empty string if the input is not a valid phone number.
func NormalizePhone(phone string) string {
	if phone == "" {
		return ""
	}
	// Parse with US as default region for numbers without country code
	num, err := phonenumbers.Parse(phone, "US")
	if err != nil {
		return ""
	}
	if !phonenumbers.IsValidNumber(num) {
		return ""
	}
	return phonenumbers.Format(num, phonenumbers.E164)
}
