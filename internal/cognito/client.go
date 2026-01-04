package cognito

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"

	dbgen "github.com/codr1/Pickleicious/internal/db/generated"
)

// ErrCognitoThrottled marks errors returned when Cognito throttles requests.
var ErrCognitoThrottled = errors.New("cognito throttling")

// ErrCognitoNotAuthorized marks errors returned when Cognito rejects credentials.
var ErrCognitoNotAuthorized = errors.New("cognito not authorized")

// ErrCognitoExpiredCode marks errors returned when Cognito sees expired codes.
var ErrCognitoExpiredCode = errors.New("cognito code expired")

// ErrCognitoCodeMismatch marks errors returned when Cognito sees mismatched codes.
var ErrCognitoCodeMismatch = errors.New("cognito code mismatch")

type CognitoClient struct {
	client       *cognitoidentityprovider.Client
	clientID     string
	clientSecret string
}

func NewClient(cfg dbgen.CognitoConfig) (*CognitoClient, error) {
	region, err := regionFromPoolID(cfg.PoolID)
	if err != nil {
		return nil, err
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &CognitoClient{
		client:       cognitoidentityprovider.NewFromConfig(awsCfg),
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
	}, nil
}

func (c *CognitoClient) InitiateCustomAuth(ctx context.Context, username string, authMethod string) (*cognitoidentityprovider.InitiateAuthOutput, error) {
	authParams := map[string]string{
		"USERNAME": username,
	}
	if authMethod != "" {
		authParams["AUTH_METHOD"] = authMethod
	}
	if c.clientSecret != "" {
		authParams["SECRET_HASH"] = c.secretHash(username)
	}

	out, err := c.client.InitiateAuth(ctx, &cognitoidentityprovider.InitiateAuthInput{
		AuthFlow:       types.AuthFlowTypeCustomAuth,
		ClientId:       aws.String(c.clientID),
		AuthParameters: authParams,
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

func (c *CognitoClient) RespondToAuthChallenge(ctx context.Context, session string, username string, code string) (*cognitoidentityprovider.RespondToAuthChallengeOutput, error) {
	respParams := map[string]string{
		"ANSWER":   code,
		"USERNAME": username,
	}
	if c.clientSecret != "" {
		respParams["SECRET_HASH"] = c.secretHash(username)
	}

	out, err := c.client.RespondToAuthChallenge(ctx, &cognitoidentityprovider.RespondToAuthChallengeInput{
		ChallengeName:      types.ChallengeNameTypeCustomChallenge,
		ClientId:           aws.String(c.clientID),
		Session:            aws.String(session),
		ChallengeResponses: respParams,
	})
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

func (c *CognitoClient) ForgotPassword(ctx context.Context, username string, authMethod string) (*cognitoidentityprovider.ForgotPasswordOutput, error) {
	input := &cognitoidentityprovider.ForgotPasswordInput{
		ClientId: aws.String(c.clientID),
		Username: aws.String(username),
	}
	if authMethod != "" {
		input.ClientMetadata = map[string]string{
			"AUTH_METHOD": authMethod,
		}
	}
	if c.clientSecret != "" {
		input.SecretHash = aws.String(c.secretHash(username))
	}

	out, err := c.client.ForgotPassword(ctx, input)
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

func (c *CognitoClient) ConfirmForgotPassword(ctx context.Context, username string, code string, newPassword string) (*cognitoidentityprovider.ConfirmForgotPasswordOutput, error) {
	input := &cognitoidentityprovider.ConfirmForgotPasswordInput{
		ClientId:         aws.String(c.clientID),
		Username:         aws.String(username),
		ConfirmationCode: aws.String(code),
		Password:         aws.String(newPassword),
	}
	if c.clientSecret != "" {
		input.SecretHash = aws.String(c.secretHash(username))
	}

	out, err := c.client.ConfirmForgotPassword(ctx, input)
	if err != nil {
		return nil, mapCognitoError(err)
	}

	return out, nil
}

func (c *CognitoClient) secretHash(username string) string {
	mac := hmac.New(sha256.New, []byte(c.clientSecret))
	mac.Write([]byte(username + c.clientID))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
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
	return err
}

func regionFromPoolID(poolID string) (string, error) {
	parts := strings.SplitN(poolID, "_", 2)
	if len(parts) < 2 || parts[0] == "" {
		return "", fmt.Errorf("invalid cognito pool id: %q", poolID)
	}
	return parts[0], nil
}
