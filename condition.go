package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// ============================================================
// CONDITION
// ============================================================

type ConditionRow struct {
	NoRawat      string
	NmPasien     string
	NoKTPPasien  string
	KdDokter     string
	NamaDokter   string
	NoKTPDokter  string
	KdPenyakit   string
	NmPenyakit   string
	StatusLanjut string
	IDEncounter  string
	IDCondition  string
}

func queryPendingConditions(db *sql.DB, tgl1, tgl2 string) ([]ConditionRow, error) {
	query := `
		SELECT reg_periksa.no_rawat, pasien.nm_pasien, pasien.no_ktp,
			reg_periksa.kd_dokter, pegawai.nama, pegawai.no_ktp as ktpdokter,
			diagnosa_pasien.kd_penyakit, penyakit.nm_penyakit,
			reg_periksa.status_lanjut,
			IFNULL(satu_sehat_encounter.id_encounter,'') as id_encounter,
			IFNULL(satu_sehat_condition.id_condition,'') as id_condition
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN pegawai ON pegawai.nik = reg_periksa.kd_dokter
		INNER JOIN diagnosa_pasien ON diagnosa_pasien.no_rawat = reg_periksa.no_rawat
		INNER JOIN penyakit ON penyakit.kd_penyakit = diagnosa_pasien.kd_penyakit
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		LEFT JOIN satu_sehat_condition ON satu_sehat_condition.no_rawat = reg_periksa.no_rawat
			AND satu_sehat_condition.kd_penyakit = diagnosa_pasien.kd_penyakit
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
			AND satu_sehat_encounter.id_encounter != ''`

	rows, err := db.Query(query, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query conditions: %w", err)
	}
	defer rows.Close()

	var results []ConditionRow
	for rows.Next() {
		var r ConditionRow
		err := rows.Scan(&r.NoRawat, &r.NmPasien, &r.NoKTPPasien,
			&r.KdDokter, &r.NamaDokter, &r.NoKTPDokter,
			&r.KdPenyakit, &r.NmPenyakit, &r.StatusLanjut,
			&r.IDEncounter, &r.IDCondition)
		if err != nil {
			log.Printf("⚠️ scan condition row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func buildConditionJSON(row ConditionRow, patientID, encounterID string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Condition",
		"clinicalStatus": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://terminology.hl7.org/CodeSystem/condition-clinical",
					"code":   "active",
				},
			},
		},
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/condition-category",
						"code":    "encounter-diagnosis",
						"display": "Encounter Diagnosis",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://hl7.org/fhir/sid/icd-10",
					"code":    row.KdPenyakit,
					"display": row.NmPenyakit,
				},
			},
		},
		"subject": map[string]interface{}{
			"reference": "Patient/" + patientID,
			"display":   row.NmPasien,
		},
		"encounter": map[string]interface{}{
			"reference": "Encounter/" + encounterID,
		},
	}
}

// ============================================================
// CONDITION HANDLERS
// ============================================================

func (a *App) handlePendingConditions(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1 = today
		tgl2 = today
	}

	rows, err := queryPendingConditions(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var pending []ConditionRow
	for _, r := range rows {
		if r.IDCondition == "" {
			pending = append(pending, r)
		}
	}

	jsonResponse(w, map[string]interface{}{
		"tgl1":          tgl1,
		"tgl2":          tgl2,
		"total":         len(rows),
		"pending_count": len(pending),
		"pending":       pending,
	})
}

func (a *App) handleSendConditions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tgl1 string `json:"tgl1"`
		Tgl2 string `json:"tgl2"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", 400)
		return
	}
	if req.Tgl1 == "" || req.Tgl2 == "" {
		jsonError(w, "tgl1 and tgl2 required", 400)
		return
	}

	rows, err := queryPendingConditions(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var results []map[string]interface{}
	sentCount := 0
	failCount := 0

	for _, row := range rows {
		if row.IDCondition != "" {
			continue // already sent
		}

		// Lookup patient
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			results = append(results, map[string]interface{}{
				"no_rawat":    row.NoRawat,
				"kd_penyakit": row.KdPenyakit,
				"status":      "failed",
				"error":       err.Error(),
			})
			failCount++
			continue
		}

		// Build and send condition
		condJSON := buildConditionJSON(row, patientID, row.IDEncounter)
		fhirID, err := a.ss.SendCondition(condJSON)
		if err != nil {
			results = append(results, map[string]interface{}{
				"no_rawat":    row.NoRawat,
				"kd_penyakit": row.KdPenyakit,
				"status":      "failed",
				"error":       err.Error(),
			})
			failCount++
			continue
		}

		// Save to DB
		_, err = a.db.Exec("INSERT INTO satu_sehat_condition (no_rawat, kd_penyakit, id_condition) VALUES (?, ?, ?)",
			row.NoRawat, row.KdPenyakit, fhirID)
		if err != nil {
			log.Printf("⚠️ save condition to DB failed: %v", err)
		}
		a.saveSendLog(row.NoRawat, "Condition", fhirID, "success", "")

		results = append(results, map[string]interface{}{
			"no_rawat":     row.NoRawat,
			"kd_penyakit":  row.KdPenyakit,
			"status":       "success",
			"id_condition": fhirID,
		})
		sentCount++
	}

	jsonResponse(w, map[string]interface{}{
		"sent":    sentCount,
		"failed":  failCount,
		"results": results,
	})
}
