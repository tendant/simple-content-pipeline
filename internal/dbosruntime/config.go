package dbosruntime

// Config holds DBOS runtime configuration
type Config struct {
	// DatabaseURL is the PostgreSQL connection string for DBOS state storage
	// Required. Example: postgresql://user:pass@localhost:5432/dbname
	DatabaseURL string

	// AppName identifies this application in DBOS
	// Required. Used for workflow isolation and logging
	AppName string

	// QueueName is the name of the workflow queue
	// Optional. Defaults to "default"
	QueueName string

	// Concurrency is the number of concurrent workers per queue
	// Optional. Defaults to 4
	Concurrency int

	// ApplicationVersion overrides the default binary hash for version matching
	// Optional. Allows multiple binaries to share workflows
	ApplicationVersion string
}

// WithDefaults fills in default values for optional fields
func (c *Config) WithDefaults() {
	if c.QueueName == "" {
		c.QueueName = "default"
	}
	if c.Concurrency == 0 {
		c.Concurrency = 4
	}
}
