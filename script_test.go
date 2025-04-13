package aistudio_test

import (
	"testing"

	"github.com/tmc/aistudio/internal/testing/scripttest"
)

func TestScripts(t *testing.T) {
	// Skip for now until script test infrastructure is stable
	// t.Skip("Skipping script tests until script test infrastructure is stable")
	scripttest.Run(t)
}
