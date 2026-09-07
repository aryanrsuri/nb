package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

//go:embed web.html
var webPage []byte

func serveNotebook(root, port string, out io.Writer) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return err
	}
	defer listener.Close()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(webPage)
	})
	mux.HandleFunc("GET /api/index", func(w http.ResponseWriter, r *http.Request) {
		n, err := scan(root, true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type note struct {
			ID string `json:"id"`
		}
		notes := make([]note, 0, len(n.Notes))
		for _, id := range n.Notes {
			notes = append(notes, note{id})
		}
		writeJSON(w, struct {
			Name  string `json:"name"`
			Notes []note `json:"notes"`
			Links []Link `json:"links"`
		}{filepath.Base(root), notes, n.Links})
	})
	mux.HandleFunc("GET /api/note", func(w http.ResponseWriter, r *http.Request) {
		n, err := scan(root, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id := r.URL.Query().Get("id")
		filename, exists := n.files[id]
		if !exists {
			http.Error(w, "Note not found", http.StatusNotFound)
			return
		}
		body, err := os.ReadFile(filename)
		if err != nil {
			http.Error(w, "Could not read note", http.StatusInternalServerError)
			return
		}
		writeJSON(w, struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}{id, string(body)})
	})
	server := http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only the local origin may use this read-only notebook server.
			if r.Host != listener.Addr().String() {
				http.Error(w, "Use the address printed by nb serve", http.StatusForbidden)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
			mux.ServeHTTP(w, r)
		}),
	}
	fmt.Fprintf(out, "nb: http://%s\n", listener.Addr())
	return server.Serve(listener)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(value)
}
