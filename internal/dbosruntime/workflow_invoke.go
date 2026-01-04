package dbosruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// WorkflowInput represents input to a DBOS workflow
type WorkflowInput struct {
	ContentID string                 `json:"content_id"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// StartWorkflowByName starts a DBOS workflow by name (language-agnostic)
// This allows triggering workflows implemented in any language (Go, Python, etc.)
func (r *Runtime) StartWorkflowByName(ctx context.Context, workflowName string, contentID string, metadata map[string]interface{}) (string, error) {
	// Generate workflow UUID
	workflowUUID := fmt.Sprintf("%s-%s-%d", workflowName, contentID, time.Now().UnixNano())

	// Create input
	input := WorkflowInput{
		ContentID: contentID,
		Metadata:  metadata,
	}

	// Serialize input to JSON
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("failed to marshal input: %w", err)
	}

	// Get database connection
	db := r.db

	// Insert workflow into dbos.workflow_status table
	// This makes it discoverable by Python workers
	query := `
		INSERT INTO dbos.workflow_status (
			workflow_uuid,
			status,
			name,
			request,
			executor_id,
			created_at,
			updated_at,
			application_version,
			application_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	now := time.Now().UnixMilli()
	_, err = db.ExecContext(ctx, query,
		workflowUUID,           // workflow_uuid
		"PENDING",              // status
		workflowName,           // name (Python function name)
		string(inputJSON),      // request
		"pending",              // executor_id
		now,                    // created_at
		now,                    // updated_at
		r.config.ApplicationVersion, // application_version
		r.config.AppName,       // application_id
	)

	if err != nil {
		return "", fmt.Errorf("failed to insert workflow: %w", err)
	}

	// Enqueue to the workflow queue
	queueQuery := `
		INSERT INTO dbos.workflow_queue (
			workflow_uuid,
			queue_name,
			created_at_epoch_ms
		) VALUES ($1, $2, $3)
	`

	_, err = db.ExecContext(ctx, queueQuery,
		workflowUUID,
		r.config.QueueName,
		now,
	)

	if err != nil {
		return "", fmt.Errorf("failed to enqueue workflow: %w", err)
	}

	return workflowUUID, nil
}

// WorkflowStatusInfo represents the status of a workflow
type WorkflowStatusInfo struct {
	WorkflowUUID string
	Status       string
	Name         string
	CreatedAt    int64
	UpdatedAt    int64
}

// GetWorkflowStatus retrieves the status of a workflow from the DBOS status table
func (r *Runtime) GetWorkflowStatus(ctx context.Context, workflowUUID string) (*WorkflowStatusInfo, error) {
	query := `
		SELECT workflow_uuid, status, name, created_at, updated_at
		FROM dbos.workflow_status
		WHERE workflow_uuid = $1
	`

	var info WorkflowStatusInfo
	err := r.db.QueryRowContext(ctx, query, workflowUUID).Scan(
		&info.WorkflowUUID,
		&info.Status,
		&info.Name,
		&info.CreatedAt,
		&info.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to query workflow status: %w", err)
	}

	return &info, nil
}
