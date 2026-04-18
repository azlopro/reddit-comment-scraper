package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds runtime credentials loaded from the environment / .env file.
type Config struct {
	DiscordWebhookURL string
}

// LoadConfig reads a .env file (if present) then pulls credentials from the
// environment. Returns an error if any required field is missing.
func LoadConfig() (*Config, error) {
	_ = godotenv.Load() // silently ignore missing .env

	cfg := &Config{
		DiscordWebhookURL: os.Getenv("DISCORD_WEBHOOK_URL"),
	}
	if cfg.DiscordWebhookURL == "" {
		return nil, fmt.Errorf("missing required env var: DISCORD_WEBHOOK_URL")
	}
	return cfg, nil
}

// Priority tiers for keyword matches.
type Priority int

const (
	PriorityHigh   Priority = iota // red    — direct tool-seeking
	PriorityMedium                 // yellow — problem-aware
	PriorityLow                    // blue   — therapist / treatment-seeking
)

// KeywordRule pairs a search phrase with its priority tier.
type KeywordRule struct {
	Phrase   string
	Priority Priority
}

// keywordRules is the ordered keyword table. HIGH rules come first so the
// first match wins and a post cannot be downgraded to a lower tier.
var keywordRules = []KeywordRule{
	// HIGH — direct tool-seeking (reply immediately)
	{"weed tracker", PriorityHigh},
	{"cannabis tracker", PriorityHigh},
	{"weed journal", PriorityHigh},
	{"cannabis app", PriorityHigh},
	{"track my weed", PriorityHigh},
	{"track my smoking", PriorityHigh},
	{"quit weed app", PriorityHigh},
	{"cannabis use app", PriorityHigh},
	{"smoking tracker", PriorityHigh},
	{"weed log", PriorityHigh},
	{"weed tracking app", PriorityHigh},
	{"cannabis tracking app", PriorityHigh},
	{"marijuana tracker", PriorityHigh},
	{"daily weed tracker", PriorityHigh},
	{"weed usage tracker", PriorityHigh},
	{"track cannabis consumption", PriorityHigh},
	{"cannabis consumption log", PriorityHigh},
	{"weed habit tracker", PriorityHigh},
	{"quit smoking weed app", PriorityHigh},
	{"stop weed app", PriorityHigh},
	{"cannabis sobriety app", PriorityHigh},

	// MEDIUM — problem-aware (reply with empathy, mention tool if natural)
	{"how much do i smoke", PriorityMedium},
	{"don't know how much i use", PriorityMedium},
	{"want to cut back weed", PriorityMedium},
	{"reduce cannabis", PriorityMedium},
	{"weed making me anxious", PriorityMedium},
	{"can't tell if i'm addicted", PriorityMedium},
	{"between therapy sessions", PriorityMedium},
	{"therapist cannabis", PriorityMedium},
	{"counselor weed", PriorityMedium},
	{"smoking too much weed", PriorityMedium},
	{"using too much cannabis", PriorityMedium},
	{"weed is affecting my", PriorityMedium},
	{"cannabis is ruining", PriorityMedium},
	{"cut down on weed", PriorityMedium},
	{"reduce my weed use", PriorityMedium},
	{"moderation with weed", PriorityMedium},
	{"weed tolerance", PriorityMedium},
	{"weed withdrawal", PriorityMedium},
	{"cannabis withdrawal", PriorityMedium},
	{"how to quit weed", PriorityMedium},
	{"quitting cannabis", PriorityMedium},
	{"addicted to weed", PriorityMedium},
	{"weed addiction", PriorityMedium},
	{"cannabis addiction", PriorityMedium},
	{"am i addicted to weed", PriorityMedium},

	// LOW — therapist / treatment-seeking (suggest app + counselor matching)
	{"find therapist cannabis", PriorityLow},
	{"cannabis counselor", PriorityLow},
	{"weed therapist", PriorityLow},
	{"treatment for weed", PriorityLow},
	{"outpatient cannabis", PriorityLow},
	{"cud treatment", PriorityLow},
	{"cannabis use disorder help", PriorityLow},
	{"cannabis addiction treatment", PriorityLow},
	{"weed addiction help", PriorityLow},
	{"help for cannabis addiction", PriorityLow},
	{"cannabis use disorder", PriorityLow},
	{"cud help", PriorityLow},
	{"marijuana addiction counselor", PriorityLow},
	{"find help for weed", PriorityLow},
	{"weed support group", PriorityLow},
	{"cannabis recovery", PriorityLow},
	{"recovering from weed", PriorityLow},
}

// PrimarySubreddit marks cannabis-specific subreddits that warrant comment feed
// polling. Secondary (general) subreddits only get the post feed to save quota.
var PrimarySubreddit = map[string]bool{
	"leaves":           true,
	"Petioles":         true,
	"CannabisAddiction": true,
	"quittingweed":     true,
}

// Subreddits is the ordered list of subreddits to monitor.
var Subreddits = []string{
	// Primary — highest intent
	"leaves",
	"Petioles",
	"CannabisAddiction",
	"quittingweed",
	// Secondary — broader but relevant
	"Mindfulness",
	"DecidingToBeBetter",
	"addiction",
	"mentalhealth",
	"selfimprovement",
	"addictionrecovery",
	"GetMotivated",
}

// negativeKeywords are phrases that indicate the post is not from someone
// actively seeking help — excluded even if a positive rule also matches.
var negativeKeywords = []string{
	// Recreational / pro-use content
	"strain", "strains", "grow", "growing", "harvest", "dispensary",
	"getting high", "smoke session", "blaze", "420", "edibles review",
	"best weed", "favorite strain", "recreational",
	// Already-quit / success stories — person is past the problem, not seeking help
	"changed my life", "changed my world",
	"months clean", "days clean", "weeks clean", "years clean",
	"month sober", "months sober", "days sober", "years sober",
	"hit sobriety", "sobriety milestone",
	"tips for recovery", "here are my tips", "here are some tips",
	"i'm not an expert but", "im not an expert but",
	"advice for quitters", "advice for new quitters",
	// Advice-giving / motivational posts (not help-seeking)
	"my advice", "my tips", "lessons learned", "what worked for me",
}

// PriorityColor returns the Discord embed color (decimal integer) for a tier.
func PriorityColor(p Priority) int {
	switch p {
	case PriorityHigh:
		return 0xE74C3C // red
	case PriorityMedium:
		return 0xF1C40F // yellow
	default:
		return 0x3498DB // blue
	}
}

// PriorityLabel returns a short human-readable label for the priority tier.
func PriorityLabel(p Priority) string {
	switch p {
	case PriorityHigh:
		return "HIGH"
	case PriorityMedium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}
