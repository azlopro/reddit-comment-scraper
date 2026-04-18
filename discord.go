package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DiscordEmbed is the structure of a single Discord embed object.
type DiscordEmbed struct {
	Title       string       `json:"title"`
	URL         string       `json:"url"`
	Color       int          `json:"color"`
	Description string       `json:"description,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
	Timestamp   string       `json:"timestamp,omitempty"` // RFC3339
}

// EmbedField is a single name/value field inside a Discord embed.
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// EmbedFooter is the footer section of a Discord embed.
type EmbedFooter struct {
	Text string `json:"text"`
}

// WebhookPayload is the JSON body sent to a Discord webhook URL.
type WebhookPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

// BuildEmbed constructs a rich Discord embed from a MatchResult.
func BuildEmbed(m MatchResult) DiscordEmbed {
	title := m.Title
	if len(title) > 253 {
		title = title[:253] + "…"
	}

	desc := m.Body
	if len(desc) > 300 {
		desc = desc[:300] + "…"
	}

	return DiscordEmbed{
		Title: title,
		URL:   m.URL,
		Color: PriorityColor(m.Priority),
		Description: desc,
		Fields: []EmbedField{
			{Name: "Priority", Value: PriorityLabel(m.Priority), Inline: true},
			{Name: "Type", Value: m.Type, Inline: true},
			{Name: "Subreddit", Value: "r/" + m.Subreddit, Inline: true},
			{Name: "Author", Value: "u/" + m.Author, Inline: true},
			{Name: "Keyword", Value: m.Keyword, Inline: true},
		},
		Footer:    &EmbedFooter{Text: "SmokingTracker Monitor"},
		Timestamp: m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// SendInfoEmbed posts a plain informational embed (no match fields) to the webhook.
func SendInfoEmbed(webhookURL, title, description string) error {
	embed := DiscordEmbed{
		Title:       title,
		Color:       0xE67E22, // orange — warning
		Description: description,
		Footer:      &EmbedFooter{Text: "SmokingTracker Monitor"},
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
	body, err := json.Marshal(WebhookPayload{Embeds: []DiscordEmbed{embed}})
	if err != nil {
		return fmt.Errorf("marshal info embed: %w", err)
	}
	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("post info embed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return nil
}

// SendWebhook posts a MatchResult as a Discord embed to the given webhook URL.
// A non-2xx response is returned as an error. Transient failures are logged by
// the caller; this function does not retry.
func SendWebhook(webhookURL string, m MatchResult) error {
	payload := WebhookPayload{
		Embeds: []DiscordEmbed{BuildEmbed(m)},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("post webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}
	return nil
}
