package resolver

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIsBrowserNavigationContextError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"missing context", errors.New("CDP Runtime.evaluate: Cannot find context with specified id"), true},
		{"missing default context", errors.New("CDP Runtime.evaluate: Cannot find default execution context"), true},
		{"destroyed context", errors.New("CDP Runtime.evaluate: Execution context was destroyed."), true},
		{"navigated target", errors.New("CDP Runtime.evaluate: Inspected target navigated or closed"), true},
		{"nil", nil, false},
		{"canceled", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"websocket timeout", errors.New("read tcp 127.0.0.1:12345->127.0.0.1:9222: i/o timeout"), false},
		{"closed websocket", errors.New("websocket: close 1006 (abnormal closure): unexpected EOF"), false},
		{"other CDP error", errors.New("CDP Runtime.evaluate: Cannot find object with given id"), false},
		{"other CDP method", errors.New("CDP Page.navigate: Cannot find context with specified id"), false},
		{"non CDP context error", errors.New("Execution context was destroyed."), false},
		{"context text in transport error", errors.New("read tcp: Execution context was destroyed.: i/o timeout"), false},
		{"context error with extra failure", errors.New("CDP Runtime.evaluate: Execution context was destroyed. i/o timeout"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isBrowserNavigationContextError(tc.err); got != tc.want {
				t.Fatalf("isBrowserNavigationContextError(%v) = %t, want %t", tc.err, got, tc.want)
			}
		})
	}
}

func TestBrowserQueueReturnsWhenContextExpires(t *testing.T) {
	manager := NewTtwidManager(nil, "", "")
	manager.browserGate <- struct{}{}
	manager.cookieGate <- struct{}{}
	for _, tc := range []struct {
		name string
		call func(context.Context) error
	}{
		{"browser", func(ctx context.Context) error {
			_, err := manager.FetchDetail(ctx, "https://www.douyin.com/video/123", "123")
			return err
		}},
		{"cookie", func(ctx context.Context) error {
			_, err := manager.Get(ctx, "https://www.douyin.com/video/123")
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- tc.call(ctx) }()
			select {
			case err := <-done:
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("error = %v, want context deadline", err)
				}
			case <-time.After(time.Second):
				t.Fatal("context deadline did not release the queued call")
			}
		})
	}
}

func TestCDPReadReturnsOnContextCancellation(t *testing.T) {
	requestArrived := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err == nil {
			close(requestArrived)
			_, _, _ = conn.ReadMessage() // 留在读取中，直到客户端取消。
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connection, err := dialCDP(ctx, "ws"+strings.TrimPrefix(server.URL, "http"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	done := make(chan error, 1)
	go func() {
		_, err := connection.Call("Runtime.evaluate", nil, 10*time.Second)
		done <- err
	}()
	select {
	case <-requestArrived:
	case <-time.After(time.Second):
		t.Fatal("CDP request did not arrive")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt CDP read")
	}
}
