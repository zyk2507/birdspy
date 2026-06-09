package main

import (
	"embed"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/url"
	"strings"
	"time"
)

//go:embed public/build/** files/bird-samples/v-2.x/** web/templates/*.html
var embeddedFiles embed.FS

type AssetManager struct {
	templates   *template.Template
	entrypoints map[string]entrypoint
	publicFS    fs.FS
}

type entrypointsFile struct {
	Entrypoints map[string]entrypoint `json:"entrypoints"`
}

type entrypoint struct {
	JS  []string `json:"js"`
	CSS []string `json:"css"`
}

func NewAssetManager() (*AssetManager, error) {
	funcs := template.FuncMap{
		"date": func(t *time.Time) string {
			if t == nil {
				return "n/a"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"dateTime": func(t time.Time) string {
			if t.IsZero() {
				return "n/a"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"uptime": func(server RouteServer) int {
			return server.UptimeDays(time.Now())
		},
		"serverSwitchPath": serverSwitchPath,
	}

	templates, err := template.New("").Funcs(funcs).ParseFS(embeddedFiles, "web/templates/*.html")
	if err != nil {
		return nil, err
	}

	publicFS, err := fs.Sub(embeddedFiles, "public")
	if err != nil {
		return nil, err
	}

	manager := &AssetManager{
		templates:   templates,
		entrypoints: map[string]entrypoint{},
		publicFS:    publicFS,
	}
	manager.loadEntrypoints()
	return manager, nil
}

func (m *AssetManager) loadEntrypoints() {
	raw, err := embeddedFiles.ReadFile("public/build/entrypoints.json")
	if err != nil {
		return
	}
	var parsed entrypointsFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return
	}
	m.entrypoints = parsed.Entrypoints
}

func (m *AssetManager) Tags(entries ...string) ([]string, []string) {
	css := []string{}
	js := []string{}
	seenCSS := map[string]bool{}
	seenJS := map[string]bool{}
	for _, entry := range entries {
		files, ok := m.entrypoints[entry]
		if !ok {
			continue
		}
		for _, href := range files.CSS {
			href = normalizeAssetPath(href)
			if !seenCSS[href] {
				css = append(css, href)
				seenCSS[href] = true
			}
		}
		for _, src := range files.JS {
			src = normalizeAssetPath(src)
			if !seenJS[src] {
				js = append(js, src)
				seenJS[src] = true
			}
		}
	}
	return css, js
}

func normalizeAssetPath(value string) string {
	if value == "" || strings.HasPrefix(value, "/") || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	return "/" + value
}

func serverSwitchPath(requestPath, current, next string) string {
	if current == "" || next == "" {
		return requestPath
	}
	pathPart := requestPath
	query := ""
	if idx := strings.Index(requestPath, "?"); idx >= 0 {
		pathPart = requestPath[:idx]
		query = requestPath[idx:]
	}
	pathPart = strings.Replace(pathPart, "/"+url.PathEscape(current)+"/", "/"+url.PathEscape(next)+"/", 1)
	if pathPart == requestPath && strings.HasPrefix(pathPart, "/"+url.PathEscape(current)) {
		pathPart = "/" + url.PathEscape(next) + strings.TrimPrefix(pathPart, "/"+url.PathEscape(current))
	}
	return pathPart + query
}
