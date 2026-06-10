package benchmark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestInstanceTypeIMDSv2(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/latest/api/token" {
			w.Write([]byte("tok123"))
			return
		}
		if r.URL.Path == "/latest/meta-data/instance-type" {
			if r.Header.Get("X-aws-ec2-metadata-token") != "tok123" {
				w.WriteHeader(401)
				return
			}
			w.Write([]byte("c5.xlarge"))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	got := instanceTypeFrom(context.Background(), srv.Client(), srv.URL)
	if got != "c5.xlarge" {
		t.Fatalf("want c5.xlarge, got %q", got)
	}
}

func TestInstanceTypeUnreachable(t *testing.T) {
	got := instanceTypeFrom(context.Background(), http.DefaultClient, "http://127.0.0.1:1")
	if got != "" {
		t.Fatalf("want empty on failure, got %q", got)
	}
}
