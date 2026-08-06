package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestSelectHuhBranchRejectsEmptyBranches(t *testing.T) {
	_, err := selectHuhBranch(strings.NewReader(""), &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no local branches are available") {
		t.Fatalf("selectHuhBranch() error = %v, want empty branch list error", err)
	}
}
