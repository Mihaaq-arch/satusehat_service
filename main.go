package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

// ============================================================
// CONFIG
// ============================================================

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPass     string
	DBName     string
	SSClientID string
	SSSecret   string
	SSAuthURL  string
	SSFHIRURL  string
	SSOrgID    string
	Port       string
}

func loadConfig() Config {
	godotenv.Load() // ignore error — will use env vars if no .env
	return Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "3306"),
		DBUser:     getEnv("DB_USER", "root"),
		DBPass:     getEnv("DB_PASS", ""),
		DBName:     getEnv("DB_NAME", "sik"),
		SSClientID: os.Getenv("SS_CLIENT_ID"),
		SSSecret:   os.Getenv("SS_CLIENT_SECRET"),
		SSAuthURL:  os.Getenv("SS_AUTH_URL"),
		SSFHIRURL:  os.Getenv("SS_FHIR_URL"),
		SSOrgID:    os.Getenv("SS_ORG_ID"),
		Port:       getEnv("PORT", "8089"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// ============================================================
// APP
// ============================================================

type App struct {
	db  *sql.DB
	ss  *SSClient
	cfg Config
}

// saveSendLog records every send attempt to satu_sehat_send_log
func (a *App) saveSendLog(noRawat, resourceType, fhirID, status, errMsg string) {
	_, err := a.db.Exec(`INSERT INTO satu_sehat_send_log
		(no_rawat, resource_type, fhir_id, status, error_message)
		VALUES (?, ?, ?, ?, ?)`,
		noRawat, resourceType, fhirID, status, errMsg)
	if err != nil {
		log.Printf("⚠️ save send log: %v", err)
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	err := a.db.Ping()
	dbStatus := "ok"
	if err != nil {
		dbStatus = err.Error()
	}

	token, tokenErr := a.ss.tokenMgr.GetToken()
	tokenStatus := "ok"
	if tokenErr != nil {
		tokenStatus = tokenErr.Error()
	} else if token == "" {
		tokenStatus = "empty"
	}

	jsonResponse(w, map[string]interface{}{
		"status":   "running",
		"database": dbStatus,
		"token":    tokenStatus,
		"time":     time.Now().Format(time.RFC3339),
	})
}

// ============================================================
// LOGS HANDLER
// ============================================================

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	status := r.URL.Query().Get("status")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	query := "SELECT id, no_rawat, resource_type, fhir_id, status, error_message, created_at FROM satu_sehat_send_log WHERE 1=1"
	var args []interface{}

	if tgl1 != "" && tgl2 != "" {
		query += " AND DATE(created_at) BETWEEN ? AND ?"
		args = append(args, tgl1, tgl2)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC LIMIT ?"
	limitInt, _ := strconv.Atoi(limit)
	if limitInt <= 0 {
		limitInt = 100
	}
	args = append(args, limitInt)

	rows, err := a.db.Query(query, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var id int64
		var noRawat, resType, fhirID, st, errMsg string
		var createdAt time.Time
		if err := rows.Scan(&id, &noRawat, &resType, &fhirID, &st, &errMsg, &createdAt); err != nil {
			continue
		}
		logs = append(logs, map[string]interface{}{
			"id": id, "no_rawat": noRawat, "resource_type": resType,
			"fhir_id": fhirID, "status": st, "error_message": errMsg,
			"created_at": createdAt.Format(time.RFC3339),
		})
	}

	jsonResponse(w, map[string]interface{}{
		"total": len(logs),
		"logs":  logs,
	})
}

// ============================================================
// JSON HELPERS
// ============================================================

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Simple CORS middleware
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == "OPTIONS" {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ============================================================
// MAIN
// ============================================================

func main() {
	cfg := loadConfig()

	// Connect to DB
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true",
		cfg.DBUser, cfg.DBPass, cfg.DBHost, cfg.DBPort, cfg.DBName)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("❌ DB open error: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("❌ DB ping error: %v", err)
	}
	log.Println("✅ Database connected:", cfg.DBName)

	// Auto-create send log table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS satu_sehat_send_log (
		id BIGINT AUTO_INCREMENT PRIMARY KEY,
		no_rawat VARCHAR(20) NOT NULL,
		resource_type VARCHAR(50) NOT NULL,
		fhir_id VARCHAR(100) DEFAULT '',
		status VARCHAR(20) DEFAULT 'pending',
		error_message TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		INDEX idx_no_rawat (no_rawat),
		INDEX idx_status (status)
	)`)
	if err != nil {
		log.Printf("⚠️ create send_log table: %v", err)
	} else {
		log.Println("✅ Send log table ready")
	}

	// Auto-create mera_integration_jobs table
	initJobsTable(db)

	// Init token manager and SS client
	tokenMgr := NewTokenManager(cfg)
	ssClient := NewSSClient(cfg, tokenMgr)

	app := &App{db: db, ss: ssClient, cfg: cfg}

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", app.handleDashboard)
	mux.HandleFunc("GET /api/health", app.handleHealth)
	mux.HandleFunc("GET /api/encounters/pending", app.handlePendingEncounters)
	mux.HandleFunc("POST /api/encounters/send", app.handleSendEncounters)
	mux.HandleFunc("GET /api/encounters-ranap/pending", app.handlePendingEncountersRanap)
	mux.HandleFunc("POST /api/encounters-ranap/send", app.handleSendEncountersRanap)
	mux.HandleFunc("GET /api/conditions/pending", app.handlePendingConditions)
	mux.HandleFunc("POST /api/conditions/send", app.handleSendConditions)
	mux.HandleFunc("GET /api/logs", app.handleLogs)
	mux.HandleFunc("GET /api/observations-ttv/{type}/pending", app.handlePendingTTV)
	mux.HandleFunc("POST /api/observations-ttv/{type}/send", app.handleSendTTV)
	mux.HandleFunc("GET /api/observations-lab/pending", app.handlePendingLabObs)
	mux.HandleFunc("POST /api/observations-lab/send", app.handleSendLabObs)
	mux.HandleFunc("GET /api/observations-rad/pending", app.handlePendingRadObs)
	mux.HandleFunc("POST /api/observations-rad/send", app.handleSendRadObs)
	mux.HandleFunc("GET /api/procedures/pending", app.handlePendingProcedures)
	mux.HandleFunc("POST /api/procedures/send", app.handleSendProcedures)
	mux.HandleFunc("GET /api/medication-requests/pending", app.handlePendingMedReq)
	mux.HandleFunc("POST /api/medication-requests/send", app.handleSendMedReq)
	mux.HandleFunc("GET /api/medication-dispenses/pending", app.handlePendingMedDisp)
	mux.HandleFunc("POST /api/medication-dispenses/send", app.handleSendMedDisp)
	mux.HandleFunc("GET /api/jobs", app.handleListJobs)
	mux.HandleFunc("POST /api/jobs/retry", app.handleRetryJobs)

	// Print routes
	log.Println("📋 Routes:")
	log.Println("  GET  /api/health")
	log.Println("  GET  /api/encounters/pending")
	log.Println("  POST /api/encounters/send")
	log.Println("  GET  /api/encounters-ranap/pending")
	log.Println("  POST /api/encounters-ranap/send")
	log.Println("  GET  /api/conditions/pending")
	log.Println("  POST /api/conditions/send")
	log.Println("  GET  /api/logs")

	addr := ":" + cfg.Port
	log.Printf("🚀 Satu Sehat service running on http://localhost%s", addr)

	// Startup: test token
	go func() {
		token, err := tokenMgr.GetToken()
		if err != nil {
			log.Printf("⚠️ Initial token fetch failed: %v", err)
		} else {
			log.Printf("✅ Token OK (%d chars)", len(token))
		}
	}()

	log.Fatal(http.ListenAndServe(addr, cors(mux)))
}
