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
// OBSERVATION LAB (Laboratory Results)
// ============================================================

type LabRow struct {
	NoRawat       string
	NoRM          string
	NmPasien      string
	NoKTPPasien   string
	NoOrder       string
	TglHasil      string
	JamHasil      string
	Pemeriksaan   string
	Code          string
	System        string
	Display       string
	Nilai         string
	IDTemplate    string
	IDSpecimen    string
	KdDokter      string
	NamaDokter    string
	NoKTPDokter   string
	IDEncounter   string
	IDObservation string
	KdJenisPrw    string
	Satuan        string
	NilaiRujukan  string
	Keterangan    string
}

func queryPendingLabObs(db *sql.DB, tgl1, tgl2 string) ([]LabRow, error) {
	query := `
		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			permintaan_lab.noorder, permintaan_lab.tgl_hasil, permintaan_lab.jam_hasil,
			template_laboratorium.Pemeriksaan,
			satu_sehat_mapping_lab.code, satu_sehat_mapping_lab.system, satu_sehat_mapping_lab.display,
			detail_periksa_lab.nilai, permintaan_detail_permintaan_lab.id_template,
			satu_sehat_specimen_lab.id_specimen,
			periksa_lab.kd_dokter, pegawai.nama, pegawai.no_ktp as ktppraktisi,
			satu_sehat_encounter.id_encounter,
			IFNULL(satu_sehat_observation_lab.id_observation,'') as id_observation,
			detail_periksa_lab.kd_jenis_prw,
			template_laboratorium.satuan,
			detail_periksa_lab.nilai_rujukan,
			detail_periksa_lab.keterangan
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN permintaan_lab ON permintaan_lab.no_rawat = reg_periksa.no_rawat
		INNER JOIN permintaan_detail_permintaan_lab ON permintaan_detail_permintaan_lab.noorder = permintaan_lab.noorder
		INNER JOIN template_laboratorium ON template_laboratorium.id_template = permintaan_detail_permintaan_lab.id_template
		INNER JOIN satu_sehat_mapping_lab ON satu_sehat_mapping_lab.id_template = template_laboratorium.id_template
		INNER JOIN satu_sehat_specimen_lab ON satu_sehat_specimen_lab.noorder = permintaan_detail_permintaan_lab.noorder
			AND satu_sehat_specimen_lab.id_template = permintaan_detail_permintaan_lab.id_template
			AND satu_sehat_specimen_lab.kd_jenis_prw = permintaan_detail_permintaan_lab.kd_jenis_prw
		INNER JOIN periksa_lab ON periksa_lab.no_rawat = permintaan_lab.no_rawat
			AND periksa_lab.tgl_periksa = permintaan_lab.tgl_hasil
			AND periksa_lab.jam = permintaan_lab.jam_hasil
			AND periksa_lab.dokter_perujuk = permintaan_lab.dokter_perujuk
		INNER JOIN detail_periksa_lab ON periksa_lab.no_rawat = detail_periksa_lab.no_rawat
			AND periksa_lab.tgl_periksa = detail_periksa_lab.tgl_periksa
			AND periksa_lab.jam = detail_periksa_lab.jam
		LEFT JOIN satu_sehat_observation_lab ON satu_sehat_specimen_lab.noorder = satu_sehat_observation_lab.noorder
			AND satu_sehat_specimen_lab.id_template = satu_sehat_observation_lab.id_template
			AND satu_sehat_specimen_lab.kd_jenis_prw = satu_sehat_observation_lab.kd_jenis_prw
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN pegawai ON periksa_lab.kd_dokter = pegawai.nik
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?`

	rows, err := db.Query(query, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query lab obs: %w", err)
	}
	defer rows.Close()

	var results []LabRow
	for rows.Next() {
		var r LabRow
		if err := rows.Scan(&r.NoRawat, &r.NoRM, &r.NmPasien, &r.NoKTPPasien,
			&r.NoOrder, &r.TglHasil, &r.JamHasil, &r.Pemeriksaan,
			&r.Code, &r.System, &r.Display, &r.Nilai, &r.IDTemplate,
			&r.IDSpecimen, &r.KdDokter, &r.NamaDokter, &r.NoKTPDokter,
			&r.IDEncounter, &r.IDObservation, &r.KdJenisPrw,
			&r.Satuan, &r.NilaiRujukan, &r.Keterangan); err != nil {
			log.Printf("⚠️ scan lab obs: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func buildLabObservationJSON(row LabRow, patientID, practitionerID, orgID string) map[string]interface{} {
	effectiveDateTime := row.TglHasil + "T" + row.JamHasil + "+07:00"
	valueStr := "Hasil Lab : " + row.Nilai + " " + row.Satuan + ", Nilai Rujukan : " + row.NilaiRujukan
	if row.Keterangan != "" {
		valueStr += ", Keterangan : " + row.Keterangan
	}
	return map[string]interface{}{
		"resourceType": "Observation",
		"identifier": []interface{}{
			map[string]interface{}{"system": "http://sys-ids.kemkes.go.id/observation/" + orgID, "value": row.NoOrder + "." + row.IDTemplate},
		},
		"status": "final",
		"category": []interface{}{
			map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "http://terminology.hl7.org/CodeSystem/observation-category", "code": "laboratory", "display": "Laboratory"}}},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"system": row.System, "code": row.Code, "display": row.Display}},
		},
		"subject":   map[string]interface{}{"reference": "Patient/" + patientID},
		"performer": []interface{}{map[string]interface{}{"reference": "Practitioner/" + practitionerID}},
		"encounter": map[string]interface{}{
			"reference": "Encounter/" + row.IDEncounter,
			"display":   "Hasil Pemeriksaan Lab " + row.Pemeriksaan + " No.Rawat " + row.NoRawat + ", Atas Nama Pasien " + row.NmPasien + ", No.RM " + row.NoRM,
		},
		"specimen":          map[string]interface{}{"reference": "Specimen/" + row.IDSpecimen},
		"effectiveDateTime": effectiveDateTime,
		"valueString":       valueStr,
	}
}

// ============================================================
// LAB OBSERVATION HANDLERS
// ============================================================

func (a *App) handlePendingLabObs(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1, tgl2 = today, today
	}
	rows, err := queryPendingLabObs(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var pending, sent []LabRow
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

func (a *App) handleSendLabObs(w http.ResponseWriter, r *http.Request) {
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
	rows, err := queryPendingLabObs(a.db, req.Tgl1, req.Tgl2)
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
			a.saveSendLog(row.NoRawat, "Observation_Lab", "", "skipped", "missing NIK")
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "skipped", "reason": "missing NIK"})
			failCount++
			continue
		}
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Observation_Lab", "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "failed", "error": "patient lookup: " + err.Error()})
			failCount++
			continue
		}
		practitionerID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Observation_Lab", "", "failed", "practitioner lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "failed", "error": "practitioner lookup: " + err.Error()})
			failCount++
			continue
		}
		obs := buildLabObservationJSON(row, patientID, practitionerID, a.cfg.SSOrgID)
		fhirID, err := a.ss.SendObservation(obs)
		if err != nil {
			a.saveSendLog(row.NoRawat, "Observation_Lab", "", "failed", err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "noorder": row.NoOrder, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		_, dbErr := a.db.Exec(
			"INSERT INTO satu_sehat_observation_lab (noorder, id_template, kd_jenis_prw, id_observation) VALUES (?,?,?,?)",
			row.NoOrder, row.IDTemplate, row.KdJenisPrw, fhirID)
		if dbErr != nil {
			log.Printf("⚠️ save lab observation %s: %v", fhirID, dbErr)
		}
		a.saveSendLog(row.NoRawat, "Observation_Lab", fhirID, "success", "")
		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "noorder": row.NoOrder, "pemeriksaan": row.Pemeriksaan,
			"status": "success", "fhir_id": fhirID,
		})
		sentCount++
	}
	jsonResponse(w, map[string]interface{}{"sent": sentCount, "failed": failCount, "details": results})
}
