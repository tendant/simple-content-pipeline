package workflows

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dbos-inc/dbos-transact-golang/dbos"
	"github.com/tendant/simple-content-pipeline/internal/dbosruntime"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// WorkflowContext contains context for workflow execution
type WorkflowContext struct {
	Ctx     context.Context
	Request pipeline.ProcessRequest
	RunID   string
}

// WorkflowResult contains the result of workflow execution
type WorkflowResult struct {
	Success bool
	Error   error
	Outputs map[string]interface{}
}

// Workflow defines the interface for processing workflows
type Workflow interface {
	// Execute runs the workflow
	Execute(wctx *WorkflowContext) (*WorkflowResult, error)

	// Name returns the workflow name
	Name() string
}

// WorkflowRunner executes workflows
type WorkflowRunner struct {
	workflows   map[string]Workflow
	dbosRuntime *dbosruntime.Runtime
}

// NewWorkflowRunner creates a new workflow runner with DBOS support
func NewWorkflowRunner(dbosRuntime *dbosruntime.Runtime) *WorkflowRunner {
	runner := &WorkflowRunner{
		workflows:   make(map[string]Workflow),
		dbosRuntime: dbosRuntime,
	}

	// Register the DBOS workflow function
	if dbosRuntime != nil {
		dbos.RegisterWorkflow(dbosRuntime.Context(), runner.executeWorkflowDBOS)
	}

	return runner
}

// Register registers a workflow
func (r *WorkflowRunner) Register(job string, workflow Workflow) {
	r.workflows[job] = workflow
}

// Run executes a workflow for the given job type (synchronous - for backward compat)
func (r *WorkflowRunner) Run(wctx *WorkflowContext) (*WorkflowResult, error) {
	workflow, ok := r.workflows[wctx.Request.Job]
	if !ok {
		return &WorkflowResult{
			Success: false,
			Error:   ErrWorkflowNotFound,
		}, ErrWorkflowNotFound
	}

	return workflow.Execute(wctx)
}

// RunAsync enqueues a workflow for async execution via DBOS
func (r *WorkflowRunner) RunAsync(ctx context.Context, req pipeline.ProcessRequest) (string, error) {
	if r.dbosRuntime == nil {
		return "", errors.New("DBOS runtime not initialized")
	}

	// Generate workflow ID for exactly-once semantics
	workflowID := fmt.Sprintf("%s-%s-%d", req.Job, req.ContentID, time.Now().UnixNano())

	// Enqueue workflow with DBOS (generic function with type parameters)
	handle, err := dbos.RunWorkflow[pipeline.ProcessRequest, *WorkflowResult](
		r.dbosRuntime.Context(),
		r.executeWorkflowDBOS,
		req,
		dbos.WithWorkflowID(workflowID),
		dbos.WithQueue(r.dbosRuntime.QueueName()),
	)
	if err != nil {
		return "", err
	}

	return handle.GetWorkflowID(), nil
}

// executeWorkflowDBOS is the DBOS workflow function that wraps existing workflows
func (r *WorkflowRunner) executeWorkflowDBOS(dbosCtx dbos.DBOSContext, req pipeline.ProcessRequest) (*WorkflowResult, error) {
	// Get workflow by job type
	workflow, ok := r.workflows[req.Job]
	if !ok {
		return &WorkflowResult{
			Success: false,
			Error:   ErrWorkflowNotFound,
		}, ErrWorkflowNotFound
	}

	// Get workflow ID from DBOS context
	workflowID, err := dbosCtx.GetWorkflowID()
	if err != nil {
		return &WorkflowResult{
			Success: false,
			Error:   err,
		}, err
	}

	// Create workflow context (DBOSContext implements context.Context)
	wctx := &WorkflowContext{
		Ctx:     dbosCtx,
		Request: req,
		RunID:   workflowID,
	}

	// Execute workflow (DBOS will checkpoint automatically)
	return workflow.Execute(wctx)
}

// WorkflowStatus represents the status of a workflow execution
type WorkflowStatus struct {
	RunID      string
	State      string // "pending", "running", "succeeded", "failed"
	StartedAt  time.Time
	FinishedAt *time.Time
	Result     *WorkflowResult
	Error      error
}

// GetStatus retrieves the status of a workflow execution using DBOS SDK
func (r *WorkflowRunner) GetStatus(ctx context.Context, runID string) (*WorkflowStatus, error) {
	if r.dbosRuntime == nil {
		return nil, errors.New("status tracking requires DBOS runtime")
	}

	// Retrieve workflow handle using DBOS SDK
	handle, err := dbos.RetrieveWorkflow[*WorkflowResult](r.dbosRuntime.Context(), runID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve workflow: %w", err)
	}

	// Get workflow status from handle
	dbosStatus, err := handle.GetStatus()
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow status: %w", err)
	}

	// Convert DBOS status to our status format
	state := mapDBOSStatus(string(dbosStatus.Status))

	// Determine finished time based on status
	var finishedAt *time.Time
	if state == "succeeded" || state == "failed" {
		finishedAt = &dbosStatus.UpdatedAt
	}

	// Extract result if available (only present after successful completion)
	var result *WorkflowResult
	if dbosStatus.Output != nil {
		if r, ok := dbosStatus.Output.(*WorkflowResult); ok {
			result = r
		}
	}

	return &WorkflowStatus{
		RunID:      runID,
		State:      state,
		StartedAt:  dbosStatus.CreatedAt,
		FinishedAt: finishedAt,
		Result:     result,
		Error:      dbosStatus.Error,
	}, nil
}

// mapDBOSStatus maps DBOS status values to our status format
func mapDBOSStatus(dbosStatus string) string {
	switch dbosStatus {
	case "PENDING":
		return "pending"
	case "ENQUEUED":
		return "pending"
	case "SUCCESS":
		return "succeeded"
	case "ERROR":
		return "failed"
	case "RETRIES_EXCEEDED":
		return "failed"
	default:
		return "running"
	}
}
