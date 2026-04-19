# reddit-monitor

A lightweight Reddit RSS watcher that sends **Discord webhook notifications** when posts or comments match configurable keyword rules — built to surface users seeking help with cannabis habit tracking for [SmokingTracker](https://azlo.pro).

## What it does

- Polls `new.rss` (and `comments.rss` for primary subreddits) for a curated list of subreddits
- Matches posts and comments against a tiered keyword table (HIGH / MEDIUM / LOW priority)
- Deduplicates across restarts via an atomic JSON store (`seen.json`)
- Sends rich Discord embeds with priority colour-coding, subreddit, author, and matched keyword
- Handles HTTP 429 rate limits with exponential back-off + Retry-After header support

## Requirements

- Go 1.22+
- A Discord incoming webhook URL

## Setup

```bash
git clone https://github.com/azlopro/reddit-comment-scraper.git
cd reddit-monitor
cp .env.example .env
# Edit .env and set DISCORD_WEBHOOK_URL
go build -o reddit-monitor .
./reddit-monitor
```

## Configuration

All configuration is via environment variables (or a `.env` file):

| Variable | Required | Description |
|---|---|---|
| `DISCORD_WEBHOOK_URL` | ✅ | Discord incoming webhook URL |

Keyword rules, subreddits, and negative filters are defined in [config.go](config.go). Edit that file to customise what gets matched and where.

## Keyword tiers

| Priority | Colour | Intent |
|---|---|---|
| HIGH | 🔴 Red | Direct tool-seeking ("weed tracker app", "cannabis journal") |
| MEDIUM | 🟡 Yellow | Problem-aware ("want to cut back weed", "weed withdrawal") |
| LOW | 🔵 Blue | Treatment/therapist-seeking ("cannabis use disorder help") |

Negative keywords (recreational content, success stories, advice posts) are excluded even when a positive rule also matches.

## Running as a service

A minimal systemd unit:

```ini
[Unit]
Description=Reddit Monitor
After=network-online.target

[Service]
WorkingDirectory=/opt/reddit-monitor
EnvironmentFile=/opt/reddit-monitor/.env
ExecStart=/opt/reddit-monitor/reddit-monitor
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

## License

MIT — see [LICENSE](LICENSE).
