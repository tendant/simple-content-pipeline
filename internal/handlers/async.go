package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/tendant/simple-content-pipeline/internal/dedupe"
	"github.com/tendant/simple-content-pipeline/internal/workflows"
	"github.com/tendant/simple-content-pipeline/pkg/pipeline"
)

// AsyncHandler handles asynchronous workflow requests
type AsyncHandler struct {
	workflowRunner *workflows.WorkflowRunner
	dedupeTracker  *dedupe.Tracker
}

// NewAsyncHandler creates a new async handler
func NewAsyncHandler(runner *workflows.WorkflowRunner, tracker *dedupe.Tracker) *AsyncHandler {
	return &AsyncHandler{
		workflowRunner: runner,
		dedupeTracker:  tracker,
	}
}

// HandleProcessAsync handles POST /v1/process - enqueues workflow and returns immediately
func (h *AsyncHandler) HandleProcessAsync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request
	var req pipeline.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate
	if req.ContentID == "" {
		http.Error(w, "content_id is required", http.StatusBadRequest)
		return
	}
	if req.Job == "" {
		http.Error(w, "job is required", http.StatusBadRequest)
		return
	}

	log.Printf("Enqueueing workflow: content_id=%s, job=%s", req.ContentID, req.Job)

	// Record dedupe submission (track how many times this content has been submitted)
	seenCount := 0
	if h.dedupeTracker != nil {
		count, err := h.dedupeTracker.Record(r.Context(), req.ContentID, req.Job, 1)
		if err != nil {
			log.Printf("Warning: Failed to record dedupe: %v (continuing anyway)", err)
		} else {
			seenCount = count
			if seenCount > 1 {
				log.Printf("Duplicate submission detected: content_id=%s, seen_count=%d", req.ContentID, seenCount)
			}
		}
	}

	// Enqueue workflow (non-blocking)
	runID, err := h.workflowRunner.RunAsync(r.Context(), req)
	if err != nil {
		log.Printf("Failed to enqueue workflow: %v", err)
		http.Error(w, fmt.Sprintf("Failed to enqueue workflow: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Workflow enqueued successfully: run_id=%s, seen_count=%d", runID, seenCount)

	// Return immediately with 202 Accepted
	resp := pipeline.ProcessResponse{
		RunID:           runID,
		DedupeSeenCount: seenCount,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

// HandleStatus handles GET /v1/runs/{runID} - returns workflow status
func (h *AsyncHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract runID from URL path (/v1/runs/{runID})
	runID := r.URL.Path[len("/v1/runs/"):]
	if runID == "" {
		http.Error(w, "run_id is required", http.StatusBadRequest)
		return
	}

	log.Printf("Checking workflow status: run_id=%s", runID)

	// Get status
	status, err := h.workflowRunner.GetStatus(r.Context(), runID)
	if err != nil {
		log.Printf("Failed to get workflow status: %v", err)
		http.Error(w, "Workflow not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}
