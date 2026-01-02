package workflows

import (
	"context"

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
	workflows map[string]Workflow
}

// NewWorkflowRunner creates a new workflow runner
func NewWorkflowRunner() *WorkflowRunner {
	return &WorkflowRunner{
		workflows: make(map[string]Workflow),
	}
}

// Register registers a workflow
func (r *WorkflowRunner) Register(job string, workflow Workflow) {
	r.workflows[job] = workflow
}

// Run executes a workflow for the given job type
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
