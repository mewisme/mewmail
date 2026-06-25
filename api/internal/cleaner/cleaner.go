package cleaner

import (
	"context"
	"log/slog"
	"time"

	"mewmail/api/internal/database"
	"mewmail/api/internal/webhook"
)

const vacuumEvery = 24

// Cleaner periodically deletes expired emails.
type Cleaner struct {
	DB            *database.DB
	Log           *slog.Logger
	RetentionDays int
	Interval      time.Duration
	Webhook       *webhook.Client
	runCount      int
}

// New creates a Cleaner.
func New(db *database.DB, log *slog.Logger, retentionDays int, wh *webhook.Client) *Cleaner {
	return &Cleaner{
		DB:            db,
		Log:           log,
		RetentionDays: retentionDays,
		Interval:      time.Hour,
		Webhook:       wh,
	}
}

// Run starts the background cleanup loop; blocks until ctx is cancelled.
func (c *Cleaner) Run(ctx context.Context) {
	ticker := time.NewTicker(c.Interval)
	defer ticker.Stop()

	c.clean(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.clean(ctx)
		}
	}
}

func (c *Cleaner) clean(ctx context.Context) {
	cutoff := time.Now().UTC().AddDate(0, 0, -c.RetentionDays)
	n, err := c.DB.DeleteExpired(cutoff)
	if err != nil {
		c.Log.Error("cleaner delete failed", "error", err)
		return
	}
	if n > 0 {
		c.Log.Info("cleaner deleted emails", "count", n, "cutoff", cutoff.Format(time.RFC3339))
		if c.Webhook != nil {
			c.Webhook.EmailsCleaned(n, cutoff, c.RetentionDays)
		}
	}

	c.runCount++
	if c.runCount%vacuumEvery == 0 {
		if err := c.DB.Vacuum(); err != nil {
			c.Log.Error("cleaner vacuum failed", "error", err)
		} else {
			c.Log.Info("cleaner vacuum completed")
		}
	}
}

// CleanOnce runs one cleanup cycle (for tests).
func (c *Cleaner) CleanOnce(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -c.RetentionDays)
	return c.DB.DeleteExpired(cutoff)
}
