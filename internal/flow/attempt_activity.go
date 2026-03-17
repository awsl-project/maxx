package flow

import (
	"io"
	"sync"
)

type AttemptActivityObserver interface {
	NoteFirstByte()
	NoteActivity()
	CompleteResponseBody()
	DisableFirstByteTimeout()
}

func GetAttemptActivityObserver(c *Ctx) AttemptActivityObserver {
	if c == nil {
		return nil
	}
	if v, ok := c.Get(KeyAttemptActivity); ok {
		if observer, ok := v.(AttemptActivityObserver); ok {
			return observer
		}
	}
	return nil
}

func WrapResponseBody(c *Ctx, body io.ReadCloser) io.ReadCloser {
	observer := GetAttemptActivityObserver(c)
	if observer == nil || body == nil {
		return body
	}
	return &attemptActivityReadCloser{
		ReadCloser: body,
		observer:   observer,
	}
}

func DisableFirstByteTimeout(c *Ctx) {
	observer := GetAttemptActivityObserver(c)
	if observer == nil {
		return
	}
	observer.DisableFirstByteTimeout()
}

type attemptActivityReadCloser struct {
	io.ReadCloser
	observer      AttemptActivityObserver
	firstByteSeen bool
	completeOnce  sync.Once
}

func (r *attemptActivityReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		if !r.firstByteSeen {
			r.observer.NoteFirstByte()
			r.firstByteSeen = true
		}
		r.observer.NoteActivity()
	}
	if err == io.EOF {
		r.complete()
	}
	return n, err
}

func (r *attemptActivityReadCloser) Close() error {
	r.complete()
	return r.ReadCloser.Close()
}

func (r *attemptActivityReadCloser) complete() {
	r.completeOnce.Do(func() {
		r.observer.CompleteResponseBody()
	})
}
