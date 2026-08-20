package ai

import (
	"context"
)

type SummaryResult struct {
	Headline            string
	ConversationStarter string
}

type Service interface {
	GenerateSummary(ctx context.Context, bio string) (*SummaryResult, error)
	IsConfigured() bool
}
