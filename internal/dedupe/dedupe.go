package dedupe

import (
	"context"
	"database/sql"
	"fmt"
	"log"
)

// Tracker tracks duplicate workflow submissions
type Tracker struct {
	db *sql.DB
}

// NewTracker creates a new dedupe tracker
func NewTracker(db *sql.DB) (*Tracker, error) {
	tracker := &Tracker{db: db}

	// Create table if not exists
	if err := tracker.ensureTable(); err != nil {
		return nil, fmt.Errorf("failed to ensure dedupe table: %w", err)
	}

	return tracker, nil
}

// ensureTable creates the process_dedupe table if it doesn't exist
func (t *Tracker) ensureTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS process_dedupe (
			content_id TEXT PRIMARY KEY,
			pipeline TEXT,
			pipeline_version INTEGER,
			first_seen_at TIMESTAMPTZ DEFAULT NOW(),
			last_seen_at TIMESTAMPTZ DEFAULT NOW(),
			seen_count INTEGER DEFAULT 1
		)
	`

	_, err := t.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create process_dedupe table: %w", err)
	}

	log.Printf("✓ process_dedupe table ready")
	return nil
}

// Record records a workflow submission and returns the seen count
func (t *Tracker) Record(ctx context.Context, contentID string, pipeline string, pipelineVersion int) (int, error) {
	// Upsert: increment seen_count if exists, insert if not
	query := `
		INSERT INTO process_dedupe (content_id, pipeline, pipeline_version, first_seen_at, last_seen_at, seen_count)
		VALUES ($1, $2, $3, NOW(), NOW(), 1)
		ON CONFLICT (content_id) DO UPDATE
		SET last_seen_at = NOW(),
		    seen_count = process_dedupe.seen_count + 1,
		    pipeline = EXCLUDED.pipeline,
		    pipeline_version = EXCLUDED.pipeline_version
		RETURNING seen_count
	`

	var seenCount int
	err := t.db.QueryRowContext(ctx, query, contentID, pipeline, pipelineVersion).Scan(&seenCount)
	if err != nil {
		return 0, fmt.Errorf("failed to record dedupe: %w", err)
	}

	return seenCount, nil
}

// GetSeenCount retrieves the seen count for a content ID
func (t *Tracker) GetSeenCount(ctx context.Context, contentID string) (int, error) {
	query := `SELECT seen_count FROM process_dedupe WHERE content_id = $1`

	var seenCount int
	err := t.db.QueryRowContext(ctx, query, contentID).Scan(&seenCount)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("failed to get seen count: %w", err)
	}

	return seenCount, nil
}
