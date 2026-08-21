package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/observability"
)

type OpenAIService struct {
	apiKey  string
	baseURL string
	client  *http.Client
	logger  *slog.Logger
	metrics *observability.Metrics
}

func NewOpenAIService(apiKey, baseURL string, logger *slog.Logger, metrics *observability.Metrics) *OpenAIService {
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIService{
		apiKey:  apiKey,
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger:  logger,
		metrics: metrics,
	}
}

func (s *OpenAIService) IsConfigured() bool {
	return s.apiKey != ""
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature"`
	MaxTokens   int                 `json:"max_tokens"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (s *OpenAIService) GenerateSummary(ctx context.Context, bio string) (*SummaryResult, error) {
	bio = strings.TrimSpace(bio)
	if bio == "" {
		return &SummaryResult{Headline: "", ConversationStarter: ""}, nil
	}

	if !s.IsConfigured() {
		// Fallback gracefully to raw bio
		return &SummaryResult{
			Headline:            bio,
			ConversationStarter: bio,
		}, nil
	}

	prompt := fmt.Sprintf(`Given this person's bio, generate:
1. A punchy headline (under 60 chars) describing their identity/craft.
2. A natural conversation starter line (under 120 chars).

Bio: %s

Respond with JSON format: {"headline": "...", "conversation_starter": "..."}`, bio)

	reqBody := openAIChatRequest{
		Model: "gpt-4o-mini",
		Messages: []openAIChatMessage{
			{Role: "system", Content: "You are an assistant creating concise profile highlights for a nearby networking app."},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   150,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/chat/completions", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		if s.metrics != nil {
			s.metrics.ExternalAPIDuration.WithLabelValues("openai", "chat_completions", "error").Observe(duration.Seconds())
		}
		s.logger.WarnContext(ctx, "openai request failed; falling back to raw bio", slog.String("error", err.Error()))
		return &SummaryResult{Headline: bio, ConversationStarter: bio}, nil
	}
	defer resp.Body.Close()

	if s.metrics != nil {
		s.metrics.ExternalAPIDuration.WithLabelValues("openai", "chat_completions", fmt.Sprintf("%d", resp.StatusCode)).Observe(duration.Seconds())
	}

	if resp.StatusCode != http.StatusOK {
		s.logger.WarnContext(ctx, "openai returned non-200; falling back to raw bio", slog.Int("status", resp.StatusCode))
		return &SummaryResult{Headline: bio, ConversationStarter: bio}, nil
	}

	var chatResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil || len(chatResp.Choices) == 0 {
		return &SummaryResult{Headline: bio, ConversationStarter: bio}, nil
	}

	content := chatResp.Choices[0].Message.Content
	// Extract JSON from response if needed
	var result struct {
		Headline            string `json:"headline"`
		ConversationStarter string `json:"conversation_starter"`
	}

	if err := json.Unmarshal([]byte(content), &result); err != nil {
		// Fallback
		return &SummaryResult{Headline: bio, ConversationStarter: bio}, nil
	}

	return &SummaryResult{
		Headline:            result.Headline,
		ConversationStarter: result.ConversationStarter,
	}, nil
}
