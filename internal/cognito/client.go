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

// InitiateEmailOTP starts the EMAIL_OTP authentication flow.
// Returns a session token to use with VerifyEmailOTP.
func (c *CognitoClient) InitiateEmailOTP(ctx context.Context, email string) (*cognitoidentityprovider.InitiateAuthOutput, error) {
	out, err := c.client.InitiateAuth(ctx, &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow: types.AuthFlowTypeUserAuth,
		ClientId: aws.String(c.clientID),
		AuthParameters: map[string]string{
			"USERNAME":            email,
			"PREFERRED_CHALLENGE": "EMAIL_OTP",
		},
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

// VerifyEmailOTP verifies the OTP code sent to the user's email.
func (c *CognitoClient) VerifyEmailOTP(ctx context.Context, session, email, code string) (*cognitoidentityprovider.RespondToAuthChallengeOutput, error) {
	out, err := c.client.RespondToAuthChallenge(ctx, &cognitoidentityprovider.RespondToAuthChallengeInput{
		ChallengeName: types.ChallengeNameTypeEmailOtp,
		ClientId:      aws.String(c.clientID),
		Session:       aws.String(session),
		ChallengeResponses: map[string]string{
			"USERNAME":       email,
			"EMAIL_OTP_CODE": code,
		},
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

// CreateUser creates a new user in the Cognito User Pool.
// The user is created with email_verified=true and no welcome email is sent.
func (c *CognitoClient) CreateUser(ctx context.Context, email string) error {
	_, err := c.client.AdminCreateUser(ctx, &cognitoidentityprovider.AdminCreateUserInput{
		UserPoolId:    aws.String(c.poolID),
		Username:      aws.String(email),
		MessageAction: types.MessageActionTypeSuppress, // Don't send welcome email
		UserAttributes: []types.AttributeType{
			{Name: aws.String("email"), Value: aws.String(email)},
			{Name: aws.String("email_verified"), Value: aws.String("true")},
		},
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
