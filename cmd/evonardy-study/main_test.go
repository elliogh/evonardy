package main

import (
	"context"
	"testing"
)

func TestHelpExitsSuccessfully(t *testing.T) {
	if err := run(context.Background(), []string{"--help"}); err != nil {
		t.Fatal(err)
	}
}
