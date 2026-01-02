-- Create schema for simple-content
CREATE SCHEMA IF NOT EXISTS content;

-- Set search path
ALTER DATABASE pipeline_dev SET search_path TO content, public;
