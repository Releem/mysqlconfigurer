package db

import (
	"testing"
)

func TestConnection(t *testing.T) {
	// This test requires a real database connection
	// Skip in CI/CD environments without a test database
	t.Skip("Skipping test - requires database connection")
}


