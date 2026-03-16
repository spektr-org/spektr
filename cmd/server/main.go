// cmd/server/main.go
//
// Spektr HTTP Server — thin transport wrapper around the api package.
//
// Usage:
//   go run cmd/server/main.go [--port 8080] [--cors]
//
// The api package owns the contract. This file only does:
//   1. JSON decode the request body into an api.XxxRequest
//   2. Call api.Xxx(request)
//   3. JSON encode the api.Response[T] back to the client
//
// No business logic here. If you're adding logic, it belongs in the api package.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/spektr-org/spektr/api"
)

// ── Handlers ─────────────────────────────────────────────────────

// GET /health
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, api.Health())
}

// POST /discover
func discoverHandler(w http.ResponseWriter, r *http.Request) {
	var req api.DiscoverRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, api.Discover(req))
}

// POST /refine
func refineHandler(w http.ResponseWriter, r *http.Request) {
	var req api.RefineRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, api.Refine(req))
}

// POST /parse
func parseHandler(w http.ResponseWriter, r *http.Request) {
	var req api.ParseRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, api.Parse(req))
}

// POST /translate
func translateHandler(w http.ResponseWriter, r *http.Request) {
	var req api.TranslateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, api.Translate(req))
}

// POST /execute
func executeHandler(w http.ResponseWriter, r *http.Request) {
	var req api.ExecuteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, api.Execute(req))
}

// POST /pipeline
func pipelineHandler(w http.ResponseWriter, r *http.Request) {
	var req api.PipelineRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	writeJSON(w, api.Pipeline(req))
}

// ── JSON Helpers ─────────────────────────────────────────────────

func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":    false,
			"error": "Invalid JSON: " + err.Error(),
		})
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ── CORS Middleware ───────────────────────────────────────────────

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}

		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Logging Middleware ────────────────────────────────────────────

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("→ %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("← %s %s (%s)", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
	})
}

// ── Method Guard ─────────────────────────────────────────────────

func post(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(405)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":    false,
				"error": "Method not allowed. Use POST.",
			})
			return
		}
		handler(w, r)
	}
}

// ── Main ─────────────────────────────────────────────────────────

func main() {
	port := flag.Int("port", 8080, "Server port")
	enableCORS := flag.Bool("cors", true, "Enable CORS (default: on for Sheets/browser access)")
	flag.Parse()

	// Override port from env if set
	if envPort := os.Getenv("PORT"); envPort != "" {
		fmt.Sscanf(envPort, "%d", port)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/discover", post(discoverHandler))
	mux.HandleFunc("/refine", post(refineHandler))
	mux.HandleFunc("/parse", post(parseHandler))
	mux.HandleFunc("/translate", post(translateHandler))
	mux.HandleFunc("/execute", post(executeHandler))
	mux.HandleFunc("/pipeline", post(pipelineHandler))

	var handler http.Handler = mux
	if *enableCORS {
		handler = corsMiddleware(handler)
	}
	handler = loggingMiddleware(handler)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("Spektr server v%s listening on %s", api.Version, addr)
	log.Printf("CORS: %v", *enableCORS)
	log.Printf("Endpoints: /health /discover /refine /parse /translate /execute /pipeline")

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}