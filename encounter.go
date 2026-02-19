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
// ENCOUNTER
// ============================================================

type EncounterRow struct {
	TglRegistrasi string
	JamReg        string
	NoRawat       string
	NmPasien      string
	NoKTPPasien   string
	NoRKMMedis    string
	KdDokter      string
	NamaDokter    string
	NoKTPDokter   string
	KdPoli        string
	NmPoli        string
	IDLokasiSS    string
	SttsRawat     string
	StatusLanjut  string
	TglPulang     string
	IDEncounter   string // empty if not yet sent
}

func queryPendingEncounters(db *sql.DB, tgl1, tgl2 string) ([]EncounterRow, error) {
	query := `
		SELECT reg_periksa.tgl_registrasi, reg_periksa.jam_reg, reg_periksa.no_rawat,
			pasien.nm_pasien, pasien.no_ktp, reg_periksa.no_rkm_medis,
			reg_periksa.kd_dokter, pegawai.nama, pegawai.no_ktp as ktpdokter,
			reg_periksa.kd_poli, poliklinik.nm_poli,
			satu_sehat_mapping_lokasi_ralan.id_lokasi_satusehat,
			reg_periksa.stts, reg_periksa.status_lanjut,
			CONCAT(reg_periksa.tgl_registrasi,'T',reg_periksa.jam_reg,'+07:00') as pulang,
			IFNULL(satu_sehat_encounter.id_encounter,'') as id_encounter
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN pegawai ON pegawai.nik = reg_periksa.kd_dokter
		INNER JOIN poliklinik ON reg_periksa.kd_poli = poliklinik.kd_poli
		INNER JOIN satu_sehat_mapping_lokasi_ralan ON satu_sehat_mapping_lokasi_ralan.kd_poli = poliklinik.kd_poli
		LEFT JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		WHERE reg_periksa.status_bayar = 'Sudah Bayar'
			AND reg_periksa.tgl_registrasi BETWEEN ? AND ?`

	return scanEncounterRows(db, query, tgl1, tgl2)
}

func queryPendingEncountersRanap(db *sql.DB, tgl1, tgl2 string) ([]EncounterRow, error) {
	query := `
		SELECT reg_periksa.tgl_registrasi, reg_periksa.jam_reg, reg_periksa.no_rawat,
			pasien.nm_pasien, pasien.no_ktp, reg_periksa.no_rkm_medis,
			reg_periksa.kd_dokter, pegawai.nama, pegawai.no_ktp as ktpdokter,
			kamar_inap.kd_kamar, bangsal.nm_bangsal,
			satu_sehat_mapping_lokasi_ranap.id_lokasi_satusehat,
			reg_periksa.stts, reg_periksa.status_lanjut,
			CONCAT(reg_periksa.tgl_registrasi,'T',reg_periksa.jam_reg,'+07:00') as pulang,
			IFNULL(satu_sehat_encounter.id_encounter,'') as id_encounter
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN pegawai ON pegawai.nik = reg_periksa.kd_dokter
		INNER JOIN kamar_inap ON kamar_inap.no_rawat = reg_periksa.no_rawat
		INNER JOIN kamar ON kamar_inap.kd_kamar = kamar.kd_kamar
		INNER JOIN bangsal ON kamar.kd_bangsal = bangsal.kd_bangsal
		INNER JOIN satu_sehat_mapping_lokasi_ranap ON satu_sehat_mapping_lokasi_ranap.kd_kamar = kamar_inap.kd_kamar
		LEFT JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		WHERE reg_periksa.status_lanjut = 'Ranap'
			AND reg_periksa.tgl_registrasi BETWEEN ? AND ?`

	return scanEncounterRows(db, query, tgl1, tgl2)
}

func scanEncounterRows(db *sql.DB, query, tgl1, tgl2 string) ([]EncounterRow, error) {
	rows, err := db.Query(query, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query encounters: %w", err)
	}
	defer rows.Close()

	var results []EncounterRow
	for rows.Next() {
		var r EncounterRow
		err := rows.Scan(&r.TglRegistrasi, &r.JamReg, &r.NoRawat,
			&r.NmPasien, &r.NoKTPPasien, &r.NoRKMMedis,
			&r.KdDokter, &r.NamaDokter, &r.NoKTPDokter,
			&r.KdPoli, &r.NmPoli, &r.IDLokasiSS,
			&r.SttsRawat, &r.StatusLanjut, &r.TglPulang, &r.IDEncounter)
		if err != nil {
			log.Printf("⚠️ scan encounter row: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func buildEncounterJSON(row EncounterRow, patientID, practitionerID, orgID string) map[string]interface{} {
	classCode := "AMB"
	classDisplay := "ambulatory"
	if row.StatusLanjut != "Ralan" {
		classCode = "IMP"
		classDisplay = "inpatient encounter"
	}

	startTime := row.TglRegistrasi + "T" + row.JamReg + "+07:00"

	return map[string]interface{}{
		"resourceType": "Encounter",
		"status":       "arrived",
		"class": map[string]interface{}{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    classCode,
			"display": classDisplay,
		},
		"subject": map[string]interface{}{
			"reference": "Patient/" + patientID,
			"display":   row.NmPasien,
		},
		"participant": []interface{}{
			map[string]interface{}{
				"type": []interface{}{
					map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"system":  "http://terminology.hl7.org/CodeSystem/v3-ParticipationType",
								"code":    "ATND",
								"display": "attender",
							},
						},
					},
				},
				"individual": map[string]interface{}{
					"reference": "Practitioner/" + practitionerID,
					"display":   row.NamaDokter,
				},
			},
		},
		"period": map[string]interface{}{
			"start": startTime,
		},
		"location": []interface{}{
			map[string]interface{}{
				"location": map[string]interface{}{
					"reference": "Location/" + row.IDLokasiSS,
					"display":   row.NmPoli,
				},
			},
		},
		"statusHistory": []interface{}{
			map[string]interface{}{
				"status": "arrived",
				"period": map[string]interface{}{
					"start": startTime,
					"end":   row.TglPulang,
				},
			},
		},
		"serviceProvider": map[string]interface{}{
			"reference": "Organization/" + orgID,
		},
		"identifier": []interface{}{
			map[string]interface{}{
				"system": "http://sys-ids.kemkes.go.id/encounter/" + orgID,
				"value":  row.NoRawat,
			},
		},
	}
}

// ============================================================
// ENCOUNTER HANDLERS
// ============================================================

func (a *App) handlePendingEncounters(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1 = today
		tgl2 = today
	}

	rows, err := queryPendingEncounters(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Filter: only show those not yet sent
	var pending []EncounterRow
	var sent []EncounterRow
	for _, r := range rows {
		if r.IDEncounter == "" {
			pending = append(pending, r)
		} else {
			sent = append(sent, r)
		}
	}

	jsonResponse(w, map[string]interface{}{
		"tgl1":          tgl1,
		"tgl2":          tgl2,
		"total":         len(rows),
		"pending_count": len(pending),
		"sent_count":    len(sent),
		"pending":       pending,
	})
}

func (a *App) handleSendEncounters(w http.ResponseWriter, r *http.Request) {
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

	rows, err := queryPendingEncounters(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var results []map[string]interface{}
	sentCount := 0
	failCount := 0

	for _, row := range rows {
		if row.IDEncounter != "" {
			continue // already sent
		}
		if row.NoKTPPasien == "" || row.NoKTPDokter == "" {
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat,
				"status":   "skipped",
				"reason":   "missing NIK pasien or dokter",
			})
			failCount++
			continue
		}

		// Lookup patient
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat,
				"status":   "failed",
				"step":     "lookup_patient",
				"error":    err.Error(),
			})
			failCount++
			continue
		}

		// Lookup practitioner
		practID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat,
				"status":   "failed",
				"step":     "lookup_practitioner",
				"error":    err.Error(),
			})
			failCount++
			continue
		}

		// Build and send encounter
		encJSON := buildEncounterJSON(row, patientID, practID, a.cfg.SSOrgID)
		fhirID, err := a.ss.SendEncounter(encJSON)
		if err != nil {
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat,
				"status":   "failed",
				"step":     "send_encounter",
				"error":    err.Error(),
			})
			failCount++
			continue
		}

		// Save to satu_sehat_encounter table (same as Khanza)
		_, err = a.db.Exec("INSERT INTO satu_sehat_encounter (no_rawat, id_encounter) VALUES (?, ?)",
			row.NoRawat, fhirID)
		if err != nil {
			log.Printf("⚠️ save encounter to DB failed: %v", err)
		}
		a.saveSendLog(row.NoRawat, "Encounter", fhirID, "success", "")

		results = append(results, map[string]interface{}{
			"no_rawat":     row.NoRawat,
			"status":       "success",
			"id_encounter": fhirID,
		})
		sentCount++
	}

	jsonResponse(w, map[string]interface{}{
		"sent":    sentCount,
		"failed":  failCount,
		"results": results,
	})
}

// ============================================================
// ENCOUNTER RANAP HANDLERS
// ============================================================

func (a *App) handlePendingEncountersRanap(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1 = today
		tgl2 = today
	}

	rows, err := queryPendingEncountersRanap(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var pending, sent []EncounterRow
	for _, r := range rows {
		if r.IDEncounter == "" {
			pending = append(pending, r)
		} else {
			sent = append(sent, r)
		}
	}

	jsonResponse(w, map[string]interface{}{
		"tgl1": tgl1, "tgl2": tgl2,
		"total": len(rows), "pending_count": len(pending), "sent_count": len(sent),
		"pending": pending,
	})
}

func (a *App) handleSendEncountersRanap(w http.ResponseWriter, r *http.Request) {
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

	rows, err := queryPendingEncountersRanap(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var results []map[string]interface{}
	sentCount, failCount := 0, 0

	for _, row := range rows {
		if row.IDEncounter != "" {
			continue
		}
		if row.NoKTPPasien == "" || row.NoKTPDokter == "" {
			a.saveSendLog(row.NoRawat, "EncounterRanap", "", "skipped", "missing NIK")
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "skipped", "reason": "missing NIK",
			})
			failCount++
			continue
		}

		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, "EncounterRanap", "", "failed", err.Error())
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "failed", "step": "lookup_patient", "error": err.Error(),
			})
			failCount++
			continue
		}

		practID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, "EncounterRanap", "", "failed", err.Error())
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "failed", "step": "lookup_practitioner", "error": err.Error(),
			})
			failCount++
			continue
		}

		encJSON := buildEncounterJSON(row, patientID, practID, a.cfg.SSOrgID)
		fhirID, err := a.ss.SendEncounter(encJSON)
		if err != nil {
			a.saveSendLog(row.NoRawat, "EncounterRanap", "", "failed", err.Error())
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "failed", "step": "send_encounter", "error": err.Error(),
			})
			failCount++
			continue
		}

		_, err = a.db.Exec("INSERT INTO satu_sehat_encounter (no_rawat, id_encounter) VALUES (?, ?)",
			row.NoRawat, fhirID)
		if err != nil {
			log.Printf("⚠️ save encounter ranap to DB failed: %v", err)
		}
		a.saveSendLog(row.NoRawat, "EncounterRanap", fhirID, "success", "")

		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "status": "success", "id_encounter": fhirID,
		})
		sentCount++
	}

	jsonResponse(w, map[string]interface{}{
		"sent": sentCount, "failed": failCount, "results": results,
	})
}
