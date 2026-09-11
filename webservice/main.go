package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"mem/webservice/internal/auth"
	"mem/webservice/internal/db"
	"mem/webservice/internal/handlers"
)

// corsMiddleware adds basic CORS headers
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Token, X-Token")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	port := flag.String("port", "8080", "HTTP server port")
	dbPath := flag.String("db", "mem.db", "SQLite database file path")
	flag.Parse()

	// Environment variable overrides
	if envPort := os.Getenv("PORT"); envPort != "" {
		*port = envPort
	}
	if envDB := os.Getenv("DB_PATH"); envDB != "" {
		*dbPath = envDB
	}

	log.Printf("⚡ Initializing Mem Web Service...")
	log.Printf("📁 Database path: %s", *dbPath)

	database, err := db.InitDB(*dbPath)
	if err != nil {
		log.Fatalf("❌ Failed to initialize database: %v", err)
	}
	defer database.Close()

	sessionManager := auth.NewSessionManager()
	apiHandler := handlers.NewAPIHandler(database)
	webHandler := handlers.NewWebHandler(database, sessionManager)

	mux := http.NewServeMux()

	// Static & Web UI
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			webHandler.ServeIndex(w, r)
			return
		}
		http.NotFound(w, r)
	})

	// Web UI Auth APIs
	mux.HandleFunc("/web/api/register", webHandler.HandleRegister)
	mux.HandleFunc("/web/api/login", webHandler.HandleLogin)
	mux.HandleFunc("/web/api/logout", webHandler.HandleLogout)
	mux.HandleFunc("/web/api/me", webHandler.HandleMe)

	// Web UI Data APIs
	mux.HandleFunc("/web/api/messages", webHandler.HandleWebMessages)
	mux.HandleFunc("/web/api/messages/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			webHandler.HandleWebDeleteMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/web/api/tokens", webHandler.HandleWebTokens)
	mux.HandleFunc("/web/api/tokens/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			webHandler.HandleWebDeleteToken(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// REST API (Token-based) for Android App, Linux Client, etc.
	mux.HandleFunc("/api/v1/ping", apiHandler.Ping)
	mux.HandleFunc("/api/v1/messages/latest", apiHandler.HandleGetLatestMessage)
	mux.HandleFunc("/api/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			apiHandler.HandlePostMessage(w, r)
		} else if r.Method == http.MethodGet {
			apiHandler.HandleGetMessages(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/v1/messages/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			apiHandler.HandleDeleteMessage(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// Wrap with CORS middleware
	handler := corsMiddleware(mux)

	addr := fmt.Sprintf(":%s", strings.TrimPrefix(*port, ":"))
	log.Printf("🚀 Mem Web Service running at http://0.0.0.0%s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}
