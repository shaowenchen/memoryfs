package storage

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDetachIOContextSurvivesParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	ioCtx, ioCancel := DetachIOContext(parent)
	defer ioCancel()

	done := make(chan error, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	go func() {
		req, _ := http.NewRequestWithContext(ioCtx, http.MethodPut, srv.URL+"/chunks/x", nil)
		_, err := http.DefaultClient.Do(req)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not complete")
	}

	if errors.Is(ioCtx.Err(), context.Canceled) {
		t.Fatal("io context was canceled")
	}
}
