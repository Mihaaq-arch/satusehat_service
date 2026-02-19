package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

// ============================================================
// INTEGRATION JOBS (mera_integration_jobs)
// ============================================================

const createJobsTableSQL = `CREATE TABLE IF NOT EXISTS mera_integration_jobs (
	id              BIGINT AUTO_INCREMENT PRIMARY KEY,
	resource_type   VARCHAR(50)  NOT NULL,
	idempotency_key VARCHAR(200) NOT NULL,
	payload         JSON         NOT NULL,
	status          VARCHAR(20)  DEFAULT 'pending',
	fhir_id         VARCHAR(100) DEFAULT '',
	error_message   TEXT,
	retry_count     INT          DEFAULT 0,
	created_at      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
	updated_at      TIMESTAMP    DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
	UNIQUE KEY uk_idemp (resource_type, idempotency_key),
	INDEX idx_status (status),
	INDEX idx_created (created_at)
)`

// createJob inserts a new job. Returns jobID, or 0 if the key already exists.
func createJob(db *sql.DB, resourceType, idempotencyKey string, payload map[string]interface{}) int64 {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("⚠️ marshal job payload: %v", err)
		return 0
	}

	res, err := db.Exec(
		`INSERT IGNORE INTO mera_integration_jobs (resource_type, idempotency_key, payload, status)
		 VALUES (?, ?, ?, 'pending')`,
		resourceType, idempotencyKey, payloadJSON)
	if err != nil {
		log.Printf("⚠️ create job: %v", err)
		return 0
	}

	id, _ := res.LastInsertId()
	if id == 0 {
		// Key already exists — skip
		return 0
	}
	return id
}

// completeJob marks a job as success with the FHIR ID
func completeJob(db *sql.DB, jobID int64, fhirID string) {
	_, err := db.Exec(
		`UPDATE mera_integration_jobs SET status='success', fhir_id=?, error_message='' WHERE id=?`,
		fhirID, jobID)
	if err != nil {
		log.Printf("⚠️ complete job %d: %v", jobID, err)
	}
}

// failJob marks a job as failed and increments retry_count
func failJob(db *sql.DB, jobID int64, errMsg string) {
	_, err := db.Exec(
		`UPDATE mera_integration_jobs SET status='failed', error_message=?, retry_count=retry_count+1 WHERE id=?`,
		errMsg, jobID)
	if err != nil {
		log.Printf("⚠️ fail job %d: %v", jobID, err)
	}
}

// ============================================================
// RETRY LOGIC
// ============================================================

func (a *App) retryOneJob(jobID int64) map[string]interface{} {
	var resourceType, payload, status string
	var retryCount int
	err := a.db.QueryRow(
		`SELECT resource_type, payload, status, retry_count FROM mera_integration_jobs WHERE id=?`, jobID,
	).Scan(&resourceType, &payload, &status, &retryCount)
	if err != nil {
		return map[string]interface{}{"id": jobID, "status": "error", "error": "job not found"}
	}
	if status == "success" {
		return map[string]interface{}{"id": jobID, "status": "skipped", "reason": "already success"}
	}
	if retryCount >= 3 {
		return map[string]interface{}{"id": jobID, "status": "skipped", "reason": "max retries (3) reached"}
	}

	// Parse payload
	var fhirPayload map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &fhirPayload); err != nil {
		return map[string]interface{}{"id": jobID, "status": "error", "error": "invalid payload"}
	}

	// Determine send method based on resource type
	var fhirID string
	var sendErr error
	switch resourceType {
	case "Encounter", "EncounterRanap":
		fhirID, sendErr = a.ss.SendEncounter(fhirPayload)
	case "Condition":
		fhirID, sendErr = a.ss.SendCondition(fhirPayload)
	case "Procedure":
		fhirID, sendErr = a.ss.SendProcedure(fhirPayload)
	case "MedicationRequest":
		fhirID, sendErr = a.ss.SendMedicationRequest(fhirPayload)
	case "MedicationDispense":
		fhirID, sendErr = a.ss.SendMedicationDispense(fhirPayload)
	default:
		// Observation types (TTV, Lab, Rad)
		fhirID, sendErr = a.ss.SendObservation(fhirPayload)
	}

	if sendErr != nil {
		failJob(a.db, jobID, sendErr.Error())
		return map[string]interface{}{"id": jobID, "status": "failed", "error": sendErr.Error(), "retry_count": retryCount + 1}
	}

	completeJob(a.db, jobID, fhirID)
	return map[string]interface{}{"id": jobID, "status": "success", "fhir_id": fhirID}
}

// ============================================================
// HANDLERS
// ============================================================

func (a *App) handleListJobs(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	status := r.URL.Query().Get("status")
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "100"
	}

	query := `SELECT id, resource_type, idempotency_key, status, fhir_id, error_message, retry_count, created_at, updated_at
		FROM mera_integration_jobs WHERE 1=1`
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

	var jobs []map[string]interface{}
	for rows.Next() {
		var id, retryCount int64
		var resType, idempKey, st, fhirID, errMsg string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &resType, &idempKey, &st, &fhirID, &errMsg, &retryCount, &createdAt, &updatedAt); err != nil {
			continue
		}
		jobs = append(jobs, map[string]interface{}{
			"id": id, "resource_type": resType, "idempotency_key": idempKey,
			"status": st, "fhir_id": fhirID, "error_message": errMsg,
			"retry_count": retryCount,
			"created_at":  createdAt.Format(time.RFC3339),
			"updated_at":  updatedAt.Format(time.RFC3339),
		})
	}

	// Count by status
	var pending, failed, success int
	for _, j := range jobs {
		switch j["status"] {
		case "pending":
			pending++
		case "failed":
			failed++
		case "success":
			success++
		}
	}

	jsonResponse(w, map[string]interface{}{
		"total": len(jobs), "pending": pending, "failed": failed, "success": success,
		"jobs": jobs,
	})
}

func (a *App) handleRetryJobs(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}

	var results []map[string]interface{}

	if req.ID > 0 {
		// Retry single job
		result := a.retryOneJob(req.ID)
		results = append(results, result)
	} else if req.Status == "failed" {
		// Retry all failed jobs (retry_count < 3)
		rows, err := a.db.Query(
			`SELECT id FROM mera_integration_jobs WHERE status='failed' AND retry_count < 3 ORDER BY created_at LIMIT 100`)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		var ids []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()

		for _, id := range ids {
			results = append(results, a.retryOneJob(id))
		}
	} else {
		jsonError(w, "provide 'id' or 'status':'failed'", 400)
		return
	}

	retried, succeeded, stillFailed := 0, 0, 0
	for _, r := range results {
		retried++
		if r["status"] == "success" {
			succeeded++
		} else if r["status"] == "failed" {
			stillFailed++
		}
	}

	jsonResponse(w, map[string]interface{}{
		"retried": retried, "succeeded": succeeded, "still_failed": stillFailed,
		"details": results,
	})
}

func initJobsTable(db *sql.DB) {
	_, err := db.Exec(createJobsTableSQL)
	if err != nil {
		log.Printf("⚠️ create mera_integration_jobs table: %v", err)
	} else {
		log.Println("✅ mera_integration_jobs table ready")
	}
}

// idempKey builds a composite idempotency key from parts
func idempKey(parts ...string) string {
	key := ""
	for i, p := range parts {
		if i > 0 {
			key += "|"
		}
		key += p
	}
	return key
}

// sendViaJob wraps the job creation + send + complete/fail flow.
// Returns (fhirID, error). If job already existed, returns ("", nil) to signal skip.
func (a *App) sendViaJob(resourceType, idempotencyKey string, payload map[string]interface{},
	sendFn func(map[string]interface{}) (string, error)) (string, error) {

	jobID := createJob(a.db, resourceType, idempotencyKey, payload)
	if jobID == 0 {
		return "", nil // already processed
	}

	fhirID, err := sendFn(payload)
	if err != nil {
		failJob(a.db, jobID, err.Error())
		return "", fmt.Errorf("%w", err)
	}

	completeJob(a.db, jobID, fhirID)
	return fhirID, nil
}
