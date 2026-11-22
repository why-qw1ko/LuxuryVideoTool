package resolver

import (
	"context"
	"errors"
	"testing"
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
