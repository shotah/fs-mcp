package server

import "testing"

func TestNew(t *testing.T) {
	t.Parallel()
	if New() == nil {
		t.Fatal("nil server")
	}
	if ServerName != "fs" {
		t.Fatalf("ServerName = %q", ServerName)
	}
}
