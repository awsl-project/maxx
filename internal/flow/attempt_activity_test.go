package flow

import (
	"io"
	"strings"
	"testing"
)

type testAttemptActivityObserver struct {
	firstByteCount int
	activityCount  int
	completeCount  int
	disableCount   int
}

func (o *testAttemptActivityObserver) NoteFirstByte()           { o.firstByteCount++ }
func (o *testAttemptActivityObserver) NoteActivity()            { o.activityCount++ }
func (o *testAttemptActivityObserver) CompleteResponseBody()    { o.completeCount++ }
func (o *testAttemptActivityObserver) DisableFirstByteTimeout() { o.disableCount++ }

func TestWrapResponseBodyCompletesOnCloseWithoutEOF(t *testing.T) {
	ctx := NewCtx(nil, nil)
	observer := &testAttemptActivityObserver{}
	ctx.Set(KeyAttemptActivity, observer)

	body := WrapResponseBody(ctx, io.NopCloser(strings.NewReader("chunk")))
	if err := body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if observer.completeCount != 1 {
		t.Fatalf("CompleteResponseBody count = %d, want 1", observer.completeCount)
	}
}

func TestWrapResponseBodyCompletesOnlyOnceAcrossEOFAndClose(t *testing.T) {
	ctx := NewCtx(nil, nil)
	observer := &testAttemptActivityObserver{}
	ctx.Set(KeyAttemptActivity, observer)

	body := WrapResponseBody(ctx, io.NopCloser(strings.NewReader("chunk")))
	if _, err := io.ReadAll(body); err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if err := body.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if observer.completeCount != 1 {
		t.Fatalf("CompleteResponseBody count = %d, want 1", observer.completeCount)
	}
}
