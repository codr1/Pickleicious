package email

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
	"github.com/rs/zerolog/log"
)

// SESClient wraps AWS SESv2 sending.
type SESClient struct {
	client *sesv2.Client
	sender string
}

// NewSESClient initializes an SES client using static credentials and region.
func NewSESClient(accessKeyID, secretAccessKey, region, sender string) (*SESClient, error) {
	if accessKeyID == "" || secretAccessKey == "" || region == "" {
		return nil, fmt.Errorf("ses credentials and region are required")
	}
	if sender == "" {
		return nil, fmt.Errorf("ses sender is required")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &SESClient{
		client: sesv2.NewFromConfig(awsCfg),
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
			Str("recipient", recipient).
			Str("subject", subject).
			Time("timestamp", time.Now().UTC()).
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
			Str("recipient", recipient).
			Str("subject", subject).
			Time("timestamp", time.Now().UTC()).
			Msg("Failed to send SES email")
		return fmt.Errorf("send ses email: %w", err)
	}

	return nil
}
