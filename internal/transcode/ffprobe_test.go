package transcode

import "testing"

func TestParseProbeOutput(t *testing.T) {
	info, err := ParseProbeOutput([]byte(`{
	  "streams": [
	    {"codec_type":"video","codec_name":"h264","width":1920,"height":1080,"avg_frame_rate":"30000/1001"},
	    {"codec_type":"audio","codec_name":"aac"}
	  ],
	  "format": {"duration":"10.5","bit_rate":"6000000","size":"123456"}
	}`))
	if err != nil {
		t.Fatalf("ParseProbeOutput() error = %v", err)
	}
	if info.VideoCodec != "h264" || info.AudioCodec != "aac" {
		t.Fatalf("codecs = %s/%s", info.VideoCodec, info.AudioCodec)
	}
	if info.Width != 1920 || info.Height != 1080 {
		t.Fatalf("resolution = %dx%d", info.Width, info.Height)
	}
	if info.BitrateKbps != 6000 {
		t.Fatalf("BitrateKbps = %d", info.BitrateKbps)
	}
	if info.SizeBytes != 123456 {
		t.Fatalf("SizeBytes = %d", info.SizeBytes)
	}
}
