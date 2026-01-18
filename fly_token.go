package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GetToken returns a Fly API token string (including "FlyV1 " prefix).
// Priority: existing env var -> create a short-lived app-scoped deploy token via flyctl.
func GetToken(app string, expiry time.Duration) (string, error) {
	if t := strings.TrimSpace(os.Getenv("FLY_API_TOKEN")); t != "" {
		return t, nil
	}
	if strings.TrimSpace(app) == "" {
		return "", errors.New("app name required to create token")
	}

	// fly tokens create deploy -a <app> --expiry <duration>
	args := []string{
		"tokens", "create", "deploy",
		"-a", app,
		"--expiry", expiry.String(),
	}

	cmd := exec.Command("fly", args...)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("flyctl failed: %w\nstderr: %s", err, strings.TrimSpace(errBuf.String()))
	}

	token := extractFlyV1Token(out.String())
	if token == "" {
		combined := strings.TrimSpace(out.String() + "\n" + errBuf.String())
		return "", fmt.Errorf("couldn't find FlyV1 token in flyctl output:\n%s", combined)
	}
	return token, nil
}

func extractFlyV1Token(s string) string {
	// flyctl often prints the token as a line starting with "FlyV1 ".
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "FlyV1 ") {
			return line // keep the prefix + space
		}
	}
	return ""
}
