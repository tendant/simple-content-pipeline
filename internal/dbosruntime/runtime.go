package dbosruntime

import (
	"context"
	"errors"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
)

// Runtime manages the DBOS runtime lifecycle
type Runtime struct {
	dbosContext dbos.DBOSContext
	queue       *dbos.WorkflowQueue
	config      Config
}

// NewRuntime creates a new DBOS runtime instance
// Returns error if DBOS_SYSTEM_DATABASE_URL is not set
func NewRuntime(ctx context.Context, cfg Config) (*Runtime, error) {
	// DBOS is always required
	if cfg.DatabaseURL == "" {
		return nil, errors.New("DBOS_SYSTEM_DATABASE_URL is required")
	}

	// Apply defaults
	cfg.WithDefaults()

	// Initialize DBOS context
	dbosCtx, err := dbos.NewDBOSContext(ctx, dbos.Config{
		DatabaseURL: cfg.DatabaseURL,
		AppName:     cfg.AppName,
	})
	if err != nil {
		return nil, err
	}

	// Create workflow queue
	queue := dbos.NewWorkflowQueue(dbosCtx, cfg.QueueName)

	return &Runtime{
		dbosContext: dbosCtx,
		queue:       &queue,
		config:      cfg,
	}, nil
}

// Launch starts the DBOS runtime and workers
func (r *Runtime) Launch() error {
	return dbos.Launch(r.dbosContext)
}

// Shutdown gracefully shuts down the DBOS runtime
func (r *Runtime) Shutdown(timeout time.Duration) error {
	dbos.Shutdown(r.dbosContext, timeout)
	return nil
}

// Context returns the DBOS context
func (r *Runtime) Context() dbos.DBOSContext {
	return r.dbosContext
}

// QueueName returns the configured queue name
func (r *Runtime) QueueName() string {
	return r.config.QueueName
}

// Concurrency returns the configured concurrency
func (r *Runtime) Concurrency() int {
	return r.config.Concurrency
}
