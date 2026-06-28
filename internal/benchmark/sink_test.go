package benchmark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPSinkPostsResult(t *testing.T) {
	var gotPath string
	var gotBody Result
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := NewHTTPSink(srv.URL, srv.Client())
	res := Result{Benchmark: true, MachineLabel: "c5.xlarge", Clip: "a.mp4", Repetition: 1,
		Renditions: []ResultRendition{{Codec: "av1", Width: 1280, Height: 720, ElapsedSeconds: 12.5}}}
	if err := sink.Post(context.Background(), res); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/benchmark-runs" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotBody.MachineLabel != "c5.xlarge" || len(gotBody.Renditions) != 1 || gotBody.Renditions[0].Codec != "av1" {
		t.Fatalf("body not posted correctly: %#v", gotBody)
	}
}

func TestResultMarshalsSessionID(t *testing.T) {
	r := Result{SessionID: "123e4567-e89b-42d3-a456-426614174000", MachineLabel: "c5.xlarge"}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"sessionId":"123e4567-e89b-42d3-a456-426614174000"`) {
		t.Fatalf("JSON sem sessionId: %s", b)
	}
}

func TestResultOmitsEmptySessionID(t *testing.T) {
	r := Result{MachineLabel: "c5.xlarge"}
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), "sessionId") {
		t.Fatalf("sessionId vazio não deveria aparecer: %s", b)
	}
}
