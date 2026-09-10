package service

import (
	"context"
	"testing"
	"time"
)

func TestDetachedCleanupTransitionContextRetainsParentDeadline(t *testing.T) {
	parentDeadline := time.Now().Add(time.Second)
	parent, cancelParent := context.WithDeadline(context.Background(), parentDeadline)
	defer cancelParent()

	ctx, cancel := detachedCleanupTransitionContext(parent)
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("cleanup transition context has no deadline")
	}
	if deadline.After(parentDeadline) {
		t.Fatalf("cleanup deadline %s exceeds parent deadline %s", deadline, parentDeadline)
	}

	cancelParent()
	if err := ctx.Err(); err != nil {
		t.Fatalf("cleanup transition context canceled with parent: %v", err)
	}
}
