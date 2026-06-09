package main

import (
	"crypto/md5"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

func stableKey(value string) string {
	sum := md5.Sum([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func splitLines(data string) []string {
	data = strings.ReplaceAll(data, "\r\n", "\n")
	data = strings.ReplaceAll(data, "\r", "\n")
	return strings.Split(data, "\n")
}

func stripRegexDelimiters(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if len(pattern) >= 2 && strings.HasPrefix(pattern, "/") {
		last := strings.LastIndex(pattern, "/")
		if last > 0 {
			return pattern[1:last]
		}
	}
	return pattern
}

func compilePatterns(patterns map[string]string, flags map[string]Flag) []flagPattern {
	keys := make([]string, 0, len(patterns))
	for key := range patterns {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	results := []flagPattern{}
	for _, key := range keys {
		flag, ok := flags[key]
		if !ok {
			continue
		}
		re, err := regexp.Compile(stripRegexDelimiters(patterns[key]))
		if err != nil {
			continue
		}
		results = append(results, flagPattern{regexp: re, flag: flag})
	}
	return results
}

func htmlAttr(value string) string {
	return html.EscapeString(value)
}

func pathWithQuery(base string, values url.Values) string {
	query := values.Encode()
	if query == "" {
		return base
	}
	return base + "?" + query
}

type jsonResponse struct {
	Data      any   `json:"data"`
	Timestamp int64 `json:"timestamp"`
}
