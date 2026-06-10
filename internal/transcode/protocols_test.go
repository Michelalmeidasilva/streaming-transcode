package transcode

import (
	"reflect"
	"testing"
)

func TestResolveProtocols(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"empty defaults to both", nil, []string{"hls", "dash"}},
		{"hls only", []string{"hls"}, []string{"hls"}},
		{"dash only", []string{"dash"}, []string{"dash"}},
		{"both explicit", []string{"hls", "dash"}, []string{"hls", "dash"}},
		{"dedupe + case", []string{"HLS", " hls ", "dash"}, []string{"hls", "dash"}},
		{"unknown dropped, falls back", []string{"smooth"}, []string{"hls", "dash"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveProtocols(c.in); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("ResolveProtocols(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestResolveSegmentSeconds(t *testing.T) {
	for in, want := range map[int]int{0: 6, 1: 6, 2: 2, 3: 6, 4: 4, 5: 6, 6: 6, 99: 6} {
		if got := ResolveSegmentSeconds(in); got != want {
			t.Fatalf("ResolveSegmentSeconds(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestHasProtocol(t *testing.T) {
	if !HasProtocol([]string{"hls"}, "hls") {
		t.Fatal("expected hls present")
	}
	if HasProtocol([]string{"dash"}, "hls") {
		t.Fatal("expected hls absent")
	}
}
