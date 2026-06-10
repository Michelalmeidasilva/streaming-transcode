package benchmark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
