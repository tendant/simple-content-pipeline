package workflows

import "errors"

var (
	// ErrWorkflowNotFound is returned when a workflow is not registered
	ErrWorkflowNotFound = errors.New("workflow not found")

	// ErrStepFailed is returned when a workflow step fails
	ErrStepFailed = errors.New("workflow step failed")

	// ErrInvalidRequest is returned when the request is invalid
	ErrInvalidRequest = errors.New("invalid workflow request")
)
