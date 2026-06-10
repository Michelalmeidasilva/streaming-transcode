package benchmark

import (
	"context"
	"io"
	"net/http"
	"time"
)

// InstanceType returns the EC2 instance type via IMDSv2, or "" if unavailable.
func InstanceType(ctx context.Context) string {
	client := &http.Client{Timeout: 2 * time.Second}
	return instanceTypeFrom(ctx, client, "http://169.254.169.254")
}

func instanceTypeFrom(ctx context.Context, client *http.Client, base string) string {
	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/latest/api/token", nil)
	if err != nil {
		return ""
	}
	tokenReq.Header.Set("X-aws-ec2-metadata-token-ttl-seconds", "60")
	tokenResp, err := client.Do(tokenReq)
	if err != nil {
		return ""
	}
	defer tokenResp.Body.Close()
	token, err := io.ReadAll(tokenResp.Body)
	if err != nil || len(token) == 0 {
		return ""
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/latest/meta-data/instance-type", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("X-aws-ec2-metadata-token", string(token))
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	return string(body)
}
