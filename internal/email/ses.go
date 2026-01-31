package email

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"
)

// SESClient wraps AWS SESv2 sending.
type SESClient struct {
	client *sesv2.Client
	sender string
}

const (
	sesIdentityCacheTTL      = 10 * time.Minute
	sesIdentityVerifyTimeout = 10 * time.Second
)

type sesIdentityCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

var verifiedSESIdentities = &sesIdentityCache{
	entries: make(map[string]time.Time),
}
var verifiedSESIdentityGroup singleflight.Group

// NewSESClient initializes an SES client using static credentials and region.
func NewSESClient(accessKeyID, secretAccessKey, region, sender string) (*SESClient, error) {
	if accessKeyID == "" || secretAccessKey == "" || region == "" {
		return nil, fmt.Errorf("ses credentials and region are required")
	}
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return nil, fmt.Errorf("ses sender is required")
	}
	addr, err := mail.ParseAddress(sender)
	if err != nil {
		return nil, fmt.Errorf("ses sender must be a valid email address: %w", err)
	}
	sender = strings.TrimSpace(addr.Address)

	initCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	awsCfg, err := awsconfig.LoadDefaultConfig(
		initCtx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	sesClient := sesv2.NewFromConfig(awsCfg)
	if err := verifySESIdentity(initCtx, sesClient, sender); err != nil {
		return nil, err
	}

	return &SESClient{
		client: sesClient,
		sender: sender,
	}, nil
}

// Send delivers a simple email to a single recipient.
func (c *SESClient) Send(ctx context.Context, recipient, subject, body string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("ses client is not initialized")
	}
	if recipient == "" {
		return fmt.Errorf("recipient is required")
	}

	input := &sesv2.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses: []string{recipient},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject)},
				Body: &types.Body{
					Text: &types.Content{Data: aws.String(body)},
				},
			},
		},
		FromEmailAddress: aws.String(c.sender),
	}

	if _, err := c.client.SendEmail(ctx, input); err != nil {
		log.Error().
			Err(err).
			Str("recipient_masked", maskEmail(recipient)).
			Int("subject_len", len(subject)).
			Msg("Failed to send SES email")
		return fmt.Errorf("send ses email: %w", err)
	}

	return nil
}

// SendFrom delivers a simple email using an optional sender override.
func (c *SESClient) SendFrom(ctx context.Context, recipient, subject, body, sender string) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("ses client is not initialized")
	}
	if recipient == "" {
		return fmt.Errorf("recipient is required")
	}

	from := strings.TrimSpace(sender)
	if from == "" {
		from = c.sender
	}
	if from == "" {
		return fmt.Errorf("sender is required")
	}
	parsedFrom, err := mail.ParseAddress(from)
	if err != nil {
		return fmt.Errorf("parse sender %q: %w", from, err)
	}
	from = strings.TrimSpace(parsedFrom.Address)
	if from == "" {
		return fmt.Errorf("sender is required")
	}
	if from != c.sender {
		if err := verifySESIdentity(ctx, c.client, from); err != nil {
			return fmt.Errorf("validate ses sender %q: %w", from, err)
		}
	}

	input := &sesv2.SendEmailInput{
		Destination: &types.Destination{
			ToAddresses: []string{recipient},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{Data: aws.String(subject)},
				Body: &types.Body{
					Text: &types.Content{Data: aws.String(body)},
				},
			},
		},
		FromEmailAddress: aws.String(from),
	}

	if _, err := c.client.SendEmail(ctx, input); err != nil {
		log.Error().
			Err(err).
			Str("recipient_masked", maskEmail(recipient)).
			Int("subject_len", len(subject)).
			Msg("Failed to send SES email")
		return fmt.Errorf("send ses email: %w", err)
	}

	return nil
}

func maskEmail(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndex(email, "@")
	if at <= 1 {
		return "***"
	}
	return email[:1] + strings.Repeat("*", at-1) + email[at:]
}

func verifySESIdentity(ctx context.Context, client *sesv2.Client, sender string) error {
	if client == nil {
		return fmt.Errorf("ses client is not initialized")
	}

	identity := strings.TrimSpace(sender)
	if identity == "" {
		return fmt.Errorf("ses sender is required")
	}
	if addr, err := mail.ParseAddress(identity); err == nil {
		identity = strings.TrimSpace(addr.Address)
		if identity == "" {
			return fmt.Errorf("ses sender is required")
		}
	}

	now := time.Now()
	if verifiedSESIdentities.isFresh(identity, now) {
		return nil
	}

	result := verifiedSESIdentityGroup.DoChan(identity, func() (any, error) {
		now := time.Now()
		if verifiedSESIdentities.isFresh(identity, now) {
			return nil, nil
		}

		verifyCtx, cancel := context.WithTimeout(context.Background(), sesIdentityVerifyTimeout)
		defer cancel()

		if err := checkSESIdentity(verifyCtx, client, identity); err == nil {
			verifiedSESIdentities.mark(identity, now)
			return nil, nil
		} else if !isSESIdentityNotFound(err) {
			return nil, fmt.Errorf("validate ses identity %q: %w", identity, err)
		}

		at := strings.LastIndex(identity, "@")
		if at <= 0 || at == len(identity)-1 {
			return nil, fmt.Errorf("ses sender %q is not a verified identity", sender)
		}

		domain := identity[at+1:]
		if err := checkSESIdentity(verifyCtx, client, domain); err != nil {
			if isSESIdentityNotFound(err) {
				return nil, fmt.Errorf("ses sender %q is not a verified identity", sender)
			}
			return nil, fmt.Errorf("validate ses identity %q: %w", domain, err)
		}

		verifiedSESIdentities.mark(identity, now)
		return nil, nil
	})

	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case res := <-result:
			return res.Err
		}
	}

	res := <-result
	return res.Err
}

func (c *sesIdentityCache) isFresh(identity string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	expiresAt, ok := c.entries[identity]
	if !ok {
		return false
	}
	if now.Before(expiresAt) {
		return true
	}
	delete(c.entries, identity)
	return false
}

func (c *sesIdentityCache) mark(identity string, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[identity] = now.Add(sesIdentityCacheTTL)
}

func checkSESIdentity(ctx context.Context, client *sesv2.Client, identity string) error {
	output, err := client.GetEmailIdentity(ctx, &sesv2.GetEmailIdentityInput{
		EmailIdentity: aws.String(identity),
	})
	if err != nil {
		return err
	}

	if output.VerificationStatus != types.VerificationStatusSuccess || !output.VerifiedForSendingStatus {
		return fmt.Errorf(
			"ses identity %q is not verified for sending (status=%s, verified=%t)",
			identity,
			output.VerificationStatus,
			output.VerifiedForSendingStatus,
		)
	}

	return nil
}

func isSESIdentityNotFound(err error) bool {
	var notFound *types.NotFoundException
	return errors.As(err, &notFound)
}
