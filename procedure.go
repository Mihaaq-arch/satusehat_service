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
// PROCEDURE
// ============================================================

type ProcedureRow struct {
	NoRawat       string
	NoRM          string
	NmPasien      string
	NoKTPPasien   string
	TglRegistrasi string
	TglPulang     string
	Stts          string
	SttsLanjut    string
	IDEncounter   string
	KodeICD9      string
	NamaProsedur  string
	IDProcedure   string
	StatusProc    string
}

func queryPendingProcedures(db *sql.DB, tgl1, tgl2 string) ([]ProcedureRow, error) {
	query := `
		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			CONCAT(reg_periksa.tgl_registrasi,'T',reg_periksa.jam_reg,'+07:00') as tgl_reg,
			CONCAT(reg_periksa.tgl_registrasi,'T',reg_periksa.jam_reg,'+07:00') as tgl_pulang,
			reg_periksa.stts, reg_periksa.status_lanjut,
			satu_sehat_encounter.id_encounter,
			prosedur_pasien.kode, icd9.deskripsi_panjang,
			IFNULL(satu_sehat_procedure.id_procedure,'') as id_procedure,
			prosedur_pasien.status
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN prosedur_pasien ON prosedur_pasien.no_rawat = reg_periksa.no_rawat
		INNER JOIN icd9 ON prosedur_pasien.kode = icd9.kode
		LEFT JOIN satu_sehat_procedure ON satu_sehat_procedure.no_rawat = prosedur_pasien.no_rawat
			AND satu_sehat_procedure.kode = prosedur_pasien.kode
			AND satu_sehat_procedure.status = prosedur_pasien.status
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?`

	rows, err := db.Query(query, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query procedures: %w", err)
	}
	defer rows.Close()

	var results []ProcedureRow
	for rows.Next() {
		var r ProcedureRow
		if err := rows.Scan(&r.NoRawat, &r.NoRM, &r.NmPasien, &r.NoKTPPasien,
			&r.TglRegistrasi, &r.TglPulang, &r.Stts, &r.SttsLanjut,
			&r.IDEncounter, &r.KodeICD9, &r.NamaProsedur,
			&r.IDProcedure, &r.StatusProc); err != nil {
			log.Printf("⚠️ scan procedure: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func buildProcedureJSON(row ProcedureRow, patientID string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Procedure",
		"status":       "completed",
		"category": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"system": "http://snomed.info/sct", "code": "103693007", "display": "Diagnostic procedure"},
			},
			"text": "Diagnostic procedure",
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"system": "http://hl7.org/fhir/sid/icd-9-cm", "code": row.KodeICD9, "display": row.NamaProsedur},
			},
		},
		"subject": map[string]interface{}{"reference": "Patient/" + patientID, "display": row.NmPasien},
		"encounter": map[string]interface{}{
			"reference": "Encounter/" + row.IDEncounter,
			"display":   "Prosedur " + row.NmPasien + " selama kunjungan/dirawat dari tanggal " + row.TglRegistrasi + " sampai " + row.TglPulang,
		},
		"performedPeriod": map[string]interface{}{"start": row.TglRegistrasi, "end": row.TglPulang},
	}
}

// ============================================================
// PROCEDURE HANDLERS
// ============================================================

func (a *App) handlePendingProcedures(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1, tgl2 = today, today
	}
	rows, err := queryPendingProcedures(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var pending, sent []ProcedureRow
	for _, row := range rows {
		if row.IDProcedure == "" {
			pending = append(pending, row)
		} else {
			sent = append(sent, row)
		}
	}
	jsonResponse(w, map[string]interface{}{
		"tgl1": tgl1, "tgl2": tgl2,
		"total": len(rows), "pending_count": len(pending), "sent_count": len(sent),
		"pending": pending,
	})
}

func (a *App) handleSendProcedures(w http.ResponseWriter, r *http.Request) {
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
	rows, err := queryPendingProcedures(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var results []map[string]interface{}
	sentCount, failCount := 0, 0
	for _, row := range rows {
		if row.IDProcedure != "" {
			continue
		}
		if row.NoKTPPasien == "" {
			a.saveSendLog(row.NoRawat, "Procedure", "", "skipped", "missing NIK pasien")
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode": row.KodeICD9, "status": "skipped", "reason": "missing NIK"})
			failCount++
			continue
		}
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Procedure", "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode": row.KodeICD9, "status": "failed", "error": "patient lookup: " + err.Error()})
			failCount++
			continue
		}
		proc := buildProcedureJSON(row, patientID)
		fhirID, err := a.sendViaJob("Procedure", idempKey(row.NoRawat, row.KodeICD9, row.StatusProc), proc, a.ss.SendProcedure)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Procedure", "", "failed", err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode": row.KodeICD9, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		if fhirID == "" {
			continue
		}
		_, dbErr := a.db.Exec(
			"INSERT INTO satu_sehat_procedure (no_rawat, kode, status, id_procedure) VALUES (?,?,?,?)",
			row.NoRawat, row.KodeICD9, row.StatusProc, fhirID)
		if dbErr != nil {
			log.Printf("⚠️ save procedure %s: %v", fhirID, dbErr)
		}
		a.saveSendLog(row.NoRawat, "Procedure", fhirID, "success", "")
		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "kode": row.KodeICD9, "prosedur": row.NamaProsedur,
			"status": "success", "fhir_id": fhirID,
		})
		sentCount++
	}
	jsonResponse(w, map[string]interface{}{"sent": sentCount, "failed": failCount, "details": results})
}
