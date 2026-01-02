package pipeline

// ProcessRequest represents a request to process content
type ProcessRequest struct {
	ContentID   string            `json:"content_id"`
	ObjectKey   string            `json:"object_key"`
	ContentHash *string           `json:"content_hash,omitempty"`
	Job         string            `json:"job"` // thumbnail, ocr, object_detection
	Versions    map[string]int    `json:"versions"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ProcessResponse represents the response from triggering processing
type ProcessResponse struct {
	RunID           string `json:"run_id"`
	DedupeSeenCount int    `json:"dedupe_seen_count"`
}

// JobType constants
const (
	JobThumbnail        = "thumbnail"
	JobOCR              = "ocr"
	JobObjectDetection  = "object_detection"
)

// DerivedType constants (match simple-content conventions)
const (
	DerivedTypeThumbnail        = "thumbnail"
	DerivedTypeOCRText          = "ocr_text"
	DerivedTypeObjectDetections = "object_detections"
)
