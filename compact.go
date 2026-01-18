package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

type FlyLog struct {
	Data []struct {
		Attributes struct {
			Timestamp string         `json:"timestamp"`
			Message   string         `json:"message"`
			Level     string         `json:"level"`
			Instance  string         `json:"instance"`
			Region    string         `json:"region"`
			Meta      map[string]any `json:"meta"`
		} `json:"attributes"`
	} `json:"data"`
}

func WriteCompactLog(w io.Writer, fl FlyLog, maxMsg int) {
	for _, item := range fl.Data {
		a := item.Attributes

		ts := normTime(a.Timestamp)
		lv := strings.ToUpper(strings.TrimSpace(a.Level))
		if lv == "" {
			lv = "INFO"
		}
		region := strings.TrimSpace(a.Region)
		if region == "" {
			region = "?"
		}
		inst := strings.TrimSpace(a.Instance)
		if inst == "" {
			inst = "?"
		}

		msg := stripANSI(a.Message)
		msg = oneLine(msg)
		if maxMsg > 0 && len(msg) > maxMsg {
			msg = msg[:maxMsg] + "…"
		}

		fmt.Fprintf(w, "%s %-5s %-4s %-8s %s\n",
			ts, lv, region, inst[:min(8, len(inst))], msg)
	}
}

func stripANSI(s string) string {
	if s == "" {
		return s
	}
	return ansiRe.ReplaceAllString(s, "")
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\n", " \\n ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimSpace(s)
}

func normTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "????-??-??T??:??:??Z"
	}
	// Fly uses RFC3339Nano like 2026-01-18T01:13:23.47022662Z
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
