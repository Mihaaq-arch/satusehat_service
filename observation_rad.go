package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// ============================================================
// OBSERVATION RADIOLOGI (Imaging Results)
// ============================================================

type RadRow struct {
	NoRawat       string
	NoRM          string
	NmPasien      string
	NoKTPPasien   string
	NoOrder       string
	TglHasil      string
	JamHasil      string
	NmPerawatan   string
	Code          string
	System        string
	Display       string
	Hasil         string
	KdJenisPrw    string
	IDSpecimen    string
	KdDokter      string
	NamaDokter    string
	NoKTPDokter   string
	IDEncounter   string
	IDObservation string
}

func queryPendingRadObs(db *sql.DB, tgl1, tgl2 string) ([]RadRow, error) {
	query := `
		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			permintaan_radiologi.noorder, permintaan_radiologi.tgl_hasil, permintaan_radiologi.jam_hasil,
			jns_perawatan_radiologi.nm_perawatan,
			satu_sehat_mapping_radiologi.code, satu_sehat_mapping_radiologi.system, satu_sehat_mapping_radiologi.display,
			hasil_radiologi.hasil,
			permintaan_pemeriksaan_radiologi.kd_jenis_prw,
			satu_sehat_specimen_radiologi.id_specimen,
			periksa_radiologi.kd_dokter, pegawai.nama, pegawai.no_ktp as ktppraktisi,
			satu_sehat_encounter.id_encounter,
			IFNULL(satu_sehat_observation_radiologi.id_observation,'') as id_observation
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN permintaan_radiologi ON permintaan_radiologi.no_rawat = reg_periksa.no_rawat
		INNER JOIN permintaan_pemeriksaan_radiologi ON permintaan_pemeriksaan_radiologi.noorder = permintaan_radiologi.noorder
		INNER JOIN jns_perawatan_radiologi ON jns_perawatan_radiologi.kd_jenis_prw = permintaan_pemeriksaan_radiologi.kd_jenis_prw
		INNER JOIN satu_sehat_mapping_radiologi ON satu_sehat_mapping_radiologi.kd_jenis_prw = jns_perawatan_radiologi.kd_jenis_prw
		INNER JOIN satu_sehat_specimen_radiologi ON satu_sehat_specimen_radiologi.noorder = permintaan_pemeriksaan_radiologi.noorder
			AND satu_sehat_specimen_radiologi.kd_jenis_prw = permintaan_pemeriksaan_radiologi.kd_jenis_prw
		INNER JOIN periksa_radiologi ON periksa_radiologi.no_rawat = permintaan_radiologi.no_rawat
			AND periksa_radiologi.tgl_periksa = permintaan_radiologi.tgl_hasil
			AND periksa_radiologi.jam = permintaan_radiologi.jam_hasil
			AND periksa_radiologi.dokter_perujuk = permintaan_radiologi.dokter_perujuk
		INNER JOIN hasil_radiologi ON periksa_radiologi.no_rawat = hasil_radiologi.no_rawat
			AND periksa_radiologi.tgl_periksa = hasil_radiologi.tgl_periksa
			AND periksa_radiologi.jam = hasil_radiologi.jam
		LEFT JOIN satu_sehat_observation_radiologi ON satu_sehat_specimen_radiologi.noorder = satu_sehat_observation_radiologi.noorder
			AND satu_sehat_specimen_radiologi.kd_jenis_prw = satu_sehat_observation_radiologi.kd_jenis_prw
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN pegawai ON periksa_radiologi.kd_dokter = pegawai.nik
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?`

	rows, err := db.Query(query, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query rad obs: %w", err)
	}
	defer rows.Close()

	var results []RadRow
	for rows.Next() {
		var r RadRow
		if err := rows.Scan(&r.NoRawat, &r.NoRM, &r.NmPasien, &r.NoKTPPasien,
			&r.NoOrder, &r.TglHasil, &r.JamHasil, &r.NmPerawatan,
			&r.Code, &r.System, &r.Display, &r.Hasil,
			&r.KdJenisPrw, &r.IDSpecimen,
			&r.KdDokter, &r.NamaDokter, &r.NoKTPDokter,
			&r.IDEncounter, &r.IDObservation); err != nil {
			log.Printf("⚠️ scan rad obs: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func buildRadObservationJSON(row RadRow, patientID, practitionerID, orgID string) map[string]interface{} {
	effectiveDateTime := row.TglHasil + "T" + row.JamHasil + "+07:00"
	hasilClean := strings.ReplaceAll(row.Hasil, "\r\n", "<br>")
	hasilClean = strings.ReplaceAll(hasilClean, "\n", "<br>")
	hasilClean = strings.ReplaceAll(hasilClean, "\t", " ")

	return map[string]interface{}{
		"resourceType": "Observation",
		"identifier": []interface{}{
			map[string]interface{}{"system": "http://sys-ids.kemkes.go.id/observation/" + orgID, "value": row.NoOrder + "." + row.KdJenisPrw},
		},
		"status": "final",
		"category": []interface{}{
			map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "http://terminology.hl7.org/CodeSystem/observation-category", "code": "imaging", "display": "Imaging"}}},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"system": row.System, "code": row.Code, "display": row.Display}},
		},
		"subject":   map[string]interface{}{"reference": "Patient/" + patientID},
		"performer": []interface{}{map[string]interface{}{"reference": "Practitioner/" + practitionerID}},
		"encounter": map[string]interface{}{
			"reference": "Encounter/" + row.IDEncounter,
			"display":   "Hasil Pemeriksaan Radiologi " + row.NmPerawatan + " No.Rawat " + row.NoRawat + ", Atas Nama Pasien " + row.NmPasien + ", No.RM " + row.NoRM,
		},
		"specimen":          map[string]interface{}{"reference": "Specimen/" + row.IDSpecimen},
		"effectiveDateTime": effectiveDateTime,
		"valueString":       hasilClean,
	}
}

// ============================================================
// RAD OBSERVATION HANDLERS
// ============================================================

func (a *App) handlePendingRadObs(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1, tgl2 = today, today
	}
	rows, err := queryPendingRadObs(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var pending, sent []RadRow
	for _, row := range rows {
		if row.IDObservation == "" {
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

func (a *App) handleSendRadObs(w http.ResponseWriter, r *http.Request) {
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
	rows, err := queryPendingRadObs(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var results []map[string]interface{}
	sentCount, failCount := 0, 0
	for _, row := range rows {
		if row.IDObservation != "" {
			continue
		}
		if row.NoKTPPasien == "" || row.NoKTPDokter == "" {
			a.saveSendLog(row.NoRawat, "Observation_Rad", "", "skipped", "missing NIK")
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "skipped", "reason": "missing NIK"})
			failCount++
			continue
		}
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Observation_Rad", "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "failed", "error": "patient lookup: " + err.Error()})
			failCount++
			continue
		}
		practitionerID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Observation_Rad", "", "failed", "practitioner lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "failed", "error": "practitioner lookup: " + err.Error()})
			failCount++
			continue
		}
		obs := buildRadObservationJSON(row, patientID, practitionerID, a.cfg.SSOrgID)
		fhirID, err := a.sendViaJob("Observation_Rad", idempKey(row.NoOrder, row.KdJenisPrw), obs, a.ss.SendObservation)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Observation_Rad", "", "failed", err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		if fhirID == "" {
			continue
		}
		_, dbErr := a.db.Exec(
			"INSERT INTO satu_sehat_observation_radiologi (noorder, kd_jenis_prw, id_observation) VALUES (?,?,?)",
			row.NoOrder, row.KdJenisPrw, fhirID)
		if dbErr != nil {
			log.Printf("⚠️ save rad observation %s: %v", fhirID, dbErr)
		}
		a.saveSendLog(row.NoRawat, "Observation_Rad", fhirID, "success", "")
		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "noorder": row.NoOrder, "pemeriksaan": row.NmPerawatan,
			"status": "success", "fhir_id": fhirID,
		})
		sentCount++
	}
	jsonResponse(w, map[string]interface{}{"sent": sentCount, "failed": failCount, "details": results})
}
