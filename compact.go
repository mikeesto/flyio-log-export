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
	Meta struct {
		NextToken string `json:"next_token"`
	} `json:"meta"`

	Data []struct {
		Attributes struct {
			Timestamp string         `json:"timestamp"`
			Message   string         `json:"message"`
			Level     string         `json:"level"`
			Meta      map[string]any `json:"meta"`
		} `json:"attributes"`
	} `json:"data"`
}

func WriteCompactLog(w io.Writer, fl FlyLog) error {
	for _, item := range fl.Data {
		a := item.Attributes

		ts := normTime(a.Timestamp)
		lv := strings.ToUpper(strings.TrimSpace(a.Level))
		if lv == "" {
			lv = "INFO"
		}

		msg := stripANSI(a.Message)
		msg = oneLine(msg)

		if _, err := fmt.Fprintf(w, "%s %-5s %s\n", ts, lv, msg); err != nil {
			return err
		}
	}
	return nil
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
	return s
}
