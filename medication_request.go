package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// MEDICATION REQUEST
// ============================================================

type MedReqRow struct {
	NoRawat      string
	NoRM         string
	NmPasien     string
	NoKTPPasien  string
	NmDokter     string
	NoKTPDokter  string
	IDEncounter  string
	ObatCode     string
	ObatSystem   string
	KodeBrng     string
	ObatDisplay  string
	FormCode     string
	FormSystem   string
	FormDisplay  string
	RouteCode    string
	RouteSystem  string
	RouteDisplay string
	DenomCode    string
	DenomSystem  string
	TglPeresepan string
	Jml          string
	IDMedication string
	AturanPakai  string
	NoResep      string
	IDMedReq     string
	NoRacik      string
	SttsLanjut   string
}

func queryPendingMedReq(db *sql.DB, tgl1, tgl2 string) ([]MedReqRow, error) {
	query := `
		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			pegawai.nama, pegawai.no_ktp as ktppraktisi, satu_sehat_encounter.id_encounter,
			satu_sehat_mapping_obat.obat_code, satu_sehat_mapping_obat.obat_system,
			resep_dokter.kode_brng, satu_sehat_mapping_obat.obat_display,
			satu_sehat_mapping_obat.form_code, satu_sehat_mapping_obat.form_system, satu_sehat_mapping_obat.form_display,
			satu_sehat_mapping_obat.route_code, satu_sehat_mapping_obat.route_system, satu_sehat_mapping_obat.route_display,
			satu_sehat_mapping_obat.denominator_code, satu_sehat_mapping_obat.denominator_system,
			CONCAT(resep_obat.tgl_peresepan,' ',resep_obat.jam_peresepan) as tgl_peresepan,
			resep_dokter.jml, satu_sehat_medication.id_medication,
			resep_dokter.aturan_pakai, resep_dokter.no_resep,
			IFNULL(satu_sehat_medicationrequest.id_medicationrequest,'') as id_medicationrequest,
			'' as no_racik, 'Ralan' as stts_lanjut
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN resep_obat ON reg_periksa.no_rawat = resep_obat.no_rawat
		INNER JOIN pegawai ON resep_obat.kd_dokter = pegawai.nik
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN resep_dokter ON resep_dokter.no_resep = resep_obat.no_resep
		INNER JOIN satu_sehat_mapping_obat ON satu_sehat_mapping_obat.kode_brng = resep_dokter.kode_brng
		INNER JOIN satu_sehat_medication ON satu_sehat_medication.kode_brng = satu_sehat_mapping_obat.kode_brng
		LEFT JOIN satu_sehat_medicationrequest ON satu_sehat_medicationrequest.no_resep = resep_dokter.no_resep
			AND satu_sehat_medicationrequest.kode_brng = resep_dokter.kode_brng
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
		  AND reg_periksa.status_lanjut = 'Ralan'

		UNION ALL

		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			pegawai.nama, pegawai.no_ktp as ktppraktisi, satu_sehat_encounter.id_encounter,
			satu_sehat_mapping_obat.obat_code, satu_sehat_mapping_obat.obat_system,
			resep_dokter.kode_brng, satu_sehat_mapping_obat.obat_display,
			satu_sehat_mapping_obat.form_code, satu_sehat_mapping_obat.form_system, satu_sehat_mapping_obat.form_display,
			satu_sehat_mapping_obat.route_code, satu_sehat_mapping_obat.route_system, satu_sehat_mapping_obat.route_display,
			satu_sehat_mapping_obat.denominator_code, satu_sehat_mapping_obat.denominator_system,
			CONCAT(resep_obat.tgl_peresepan,' ',resep_obat.jam_peresepan) as tgl_peresepan,
			resep_dokter.jml, satu_sehat_medication.id_medication,
			resep_dokter.aturan_pakai, resep_dokter.no_resep,
			IFNULL(satu_sehat_medicationrequest.id_medicationrequest,'') as id_medicationrequest,
			'' as no_racik, 'Ranap' as stts_lanjut
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN resep_obat ON reg_periksa.no_rawat = resep_obat.no_rawat
		INNER JOIN pegawai ON resep_obat.kd_dokter = pegawai.nik
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN resep_dokter ON resep_dokter.no_resep = resep_obat.no_resep
		INNER JOIN satu_sehat_mapping_obat ON satu_sehat_mapping_obat.kode_brng = resep_dokter.kode_brng
		INNER JOIN satu_sehat_medication ON satu_sehat_medication.kode_brng = satu_sehat_mapping_obat.kode_brng
		LEFT JOIN satu_sehat_medicationrequest ON satu_sehat_medicationrequest.no_resep = resep_dokter.no_resep
			AND satu_sehat_medicationrequest.kode_brng = resep_dokter.kode_brng
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
		  AND reg_periksa.status_lanjut = 'Ranap'

		UNION ALL

		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			pegawai.nama, pegawai.no_ktp as ktppraktisi, satu_sehat_encounter.id_encounter,
			satu_sehat_mapping_obat.obat_code, satu_sehat_mapping_obat.obat_system,
			resep_dokter_racikan_detail.kode_brng, satu_sehat_mapping_obat.obat_display,
			satu_sehat_mapping_obat.form_code, satu_sehat_mapping_obat.form_system, satu_sehat_mapping_obat.form_display,
			satu_sehat_mapping_obat.route_code, satu_sehat_mapping_obat.route_system, satu_sehat_mapping_obat.route_display,
			satu_sehat_mapping_obat.denominator_code, satu_sehat_mapping_obat.denominator_system,
			CONCAT(resep_obat.tgl_peresepan,' ',resep_obat.jam_peresepan) as tgl_peresepan,
			resep_dokter_racikan_detail.jml, satu_sehat_medication.id_medication,
			resep_dokter_racikan.aturan_pakai, resep_dokter_racikan.no_resep,
			IFNULL(satu_sehat_medicationrequest_racikan.id_medicationrequest,'') as id_medicationrequest,
			resep_dokter_racikan_detail.no_racik, 'Ralan' as stts_lanjut
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN resep_obat ON reg_periksa.no_rawat = resep_obat.no_rawat
		INNER JOIN pegawai ON resep_obat.kd_dokter = pegawai.nik
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN resep_dokter_racikan ON resep_dokter_racikan.no_resep = resep_obat.no_resep
		INNER JOIN resep_dokter_racikan_detail ON resep_dokter_racikan_detail.no_resep = resep_dokter_racikan.no_resep
			AND resep_dokter_racikan_detail.no_racik = resep_dokter_racikan.no_racik
		INNER JOIN satu_sehat_mapping_obat ON satu_sehat_mapping_obat.kode_brng = resep_dokter_racikan_detail.kode_brng
		INNER JOIN satu_sehat_medication ON satu_sehat_medication.kode_brng = satu_sehat_mapping_obat.kode_brng
		LEFT JOIN satu_sehat_medicationrequest_racikan ON satu_sehat_medicationrequest_racikan.no_resep = resep_dokter_racikan_detail.no_resep
			AND satu_sehat_medicationrequest_racikan.kode_brng = resep_dokter_racikan_detail.kode_brng
			AND satu_sehat_medicationrequest_racikan.no_racik = resep_dokter_racikan_detail.no_racik
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
		  AND reg_periksa.status_lanjut = 'Ralan'

		UNION ALL

		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			pegawai.nama, pegawai.no_ktp as ktppraktisi, satu_sehat_encounter.id_encounter,
			satu_sehat_mapping_obat.obat_code, satu_sehat_mapping_obat.obat_system,
			resep_dokter_racikan_detail.kode_brng, satu_sehat_mapping_obat.obat_display,
			satu_sehat_mapping_obat.form_code, satu_sehat_mapping_obat.form_system, satu_sehat_mapping_obat.form_display,
			satu_sehat_mapping_obat.route_code, satu_sehat_mapping_obat.route_system, satu_sehat_mapping_obat.route_display,
			satu_sehat_mapping_obat.denominator_code, satu_sehat_mapping_obat.denominator_system,
			CONCAT(resep_obat.tgl_peresepan,' ',resep_obat.jam_peresepan) as tgl_peresepan,
			resep_dokter_racikan_detail.jml, satu_sehat_medication.id_medication,
			resep_dokter_racikan.aturan_pakai, resep_dokter_racikan.no_resep,
			IFNULL(satu_sehat_medicationrequest_racikan.id_medicationrequest,'') as id_medicationrequest,
			resep_dokter_racikan_detail.no_racik, 'Ranap' as stts_lanjut
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN resep_obat ON reg_periksa.no_rawat = resep_obat.no_rawat
		INNER JOIN pegawai ON resep_obat.kd_dokter = pegawai.nik
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN resep_dokter_racikan ON resep_dokter_racikan.no_resep = resep_obat.no_resep
		INNER JOIN resep_dokter_racikan_detail ON resep_dokter_racikan_detail.no_resep = resep_dokter_racikan.no_resep
			AND resep_dokter_racikan_detail.no_racik = resep_dokter_racikan.no_racik
		INNER JOIN satu_sehat_mapping_obat ON satu_sehat_mapping_obat.kode_brng = resep_dokter_racikan_detail.kode_brng
		INNER JOIN satu_sehat_medication ON satu_sehat_medication.kode_brng = satu_sehat_mapping_obat.kode_brng
		LEFT JOIN satu_sehat_medicationrequest_racikan ON satu_sehat_medicationrequest_racikan.no_resep = resep_dokter_racikan_detail.no_resep
			AND satu_sehat_medicationrequest_racikan.kode_brng = resep_dokter_racikan_detail.kode_brng
			AND satu_sehat_medicationrequest_racikan.no_racik = resep_dokter_racikan_detail.no_racik
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
		  AND reg_periksa.status_lanjut = 'Ranap'`

	rows, err := db.Query(query, tgl1, tgl2, tgl1, tgl2, tgl1, tgl2, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query medication requests: %w", err)
	}
	defer rows.Close()

	var results []MedReqRow
	for rows.Next() {
		var r MedReqRow
		if err := rows.Scan(&r.NoRawat, &r.NoRM, &r.NmPasien, &r.NoKTPPasien,
			&r.NmDokter, &r.NoKTPDokter, &r.IDEncounter,
			&r.ObatCode, &r.ObatSystem, &r.KodeBrng, &r.ObatDisplay,
			&r.FormCode, &r.FormSystem, &r.FormDisplay,
			&r.RouteCode, &r.RouteSystem, &r.RouteDisplay,
			&r.DenomCode, &r.DenomSystem,
			&r.TglPeresepan, &r.Jml, &r.IDMedication,
			&r.AturanPakai, &r.NoResep, &r.IDMedReq,
			&r.NoRacik, &r.SttsLanjut); err != nil {
			log.Printf("⚠️ scan med req: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

// parseSigna parses "signa1 x signa2" from aturan_pakai, returns (dose, frequency)
func parseSigna(aturan string) (string, string) {
	parts := strings.SplitN(strings.ToLower(aturan), "x", 2)
	signa1, signa2 := "1", "1"
	if len(parts) >= 1 {
		s := strings.TrimSpace(parts[0])
		s = regexp.MustCompile(`[^0-9.]+`).ReplaceAllString(s, "")
		if s != "" {
			signa1 = s
		}
	}
	if len(parts) >= 2 {
		s := strings.TrimSpace(parts[1])
		s = regexp.MustCompile(`[^0-9.]+`).ReplaceAllString(s, "")
		if s != "" {
			signa2 = s
		}
	}
	return signa1, signa2
}

func buildMedReqJSON(row MedReqRow, patientID, practitionerID, orgID string) map[string]interface{} {
	signa1, signa2 := parseSigna(row.AturanPakai)
	signa1f, _ := strconv.ParseFloat(signa1, 64)
	signa2f, _ := strconv.ParseFloat(signa2, 64)
	jmlf, _ := strconv.ParseFloat(row.Jml, 64)

	catCode, catDisplay := "outpatient", "Outpatient"
	if row.SttsLanjut == "Ranap" {
		catCode, catDisplay = "inpatient", "Inpatient"
	}

	prescValue := row.NoResep
	if row.NoRacik != "" {
		prescValue = row.NoResep + "-" + row.NoRacik
	}

	authoredOn := strings.ReplaceAll(row.TglPeresepan, " ", "T") + "+07:00"

	return map[string]interface{}{
		"resourceType": "MedicationRequest",
		"identifier": []interface{}{
			map[string]interface{}{"system": "http://sys-ids.kemkes.go.id/prescription/" + orgID, "use": "official", "value": prescValue},
			map[string]interface{}{"system": "http://sys-ids.kemkes.go.id/prescription-item/" + orgID, "use": "official", "value": row.KodeBrng},
		},
		"status": "completed",
		"intent": "order",
		"category": []interface{}{
			map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": "http://terminology.hl7.org/CodeSystem/medicationrequest-category", "code": catCode, "display": catDisplay}}},
		},
		"medicationReference": map[string]interface{}{"reference": "Medication/" + row.IDMedication, "display": row.ObatDisplay},
		"subject":             map[string]interface{}{"reference": "Patient/" + patientID, "display": row.NmPasien},
		"encounter":           map[string]interface{}{"reference": "Encounter/" + row.IDEncounter},
		"authoredOn":          authoredOn,
		"requester":           map[string]interface{}{"reference": "Practitioner/" + practitionerID, "display": row.NmDokter},
		"dosageInstruction": []interface{}{
			map[string]interface{}{
				"sequence": 1, "patientInstruction": row.AturanPakai,
				"timing": map[string]interface{}{"repeat": map[string]interface{}{"frequency": signa2f, "period": 1, "periodUnit": "d"}},
				"route":  map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": row.RouteSystem, "code": row.RouteCode, "display": row.RouteDisplay}}},
				"doseAndRate": []interface{}{
					map[string]interface{}{"doseQuantity": map[string]interface{}{"value": signa1f, "unit": row.DenomCode, "system": row.DenomSystem, "code": row.DenomCode}},
				},
			},
		},
		"dispenseRequest": map[string]interface{}{
			"quantity":  map[string]interface{}{"value": jmlf, "unit": row.DenomCode, "system": row.DenomSystem, "code": row.DenomCode},
			"performer": map[string]interface{}{"reference": "Organization/" + orgID},
		},
	}
}

// ============================================================
// MEDICATION REQUEST HANDLERS
// ============================================================

func (a *App) handlePendingMedReq(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1, tgl2 = today, today
	}
	rows, err := queryPendingMedReq(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var pending, sent []MedReqRow
	for _, row := range rows {
		if row.IDMedReq == "" {
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

func (a *App) handleSendMedReq(w http.ResponseWriter, r *http.Request) {
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
	rows, err := queryPendingMedReq(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var results []map[string]interface{}
	sentCount, failCount := 0, 0
	for _, row := range rows {
		if row.IDMedReq != "" {
			continue
		}
		if row.NoKTPPasien == "" || row.NoKTPDokter == "" {
			a.saveSendLog(row.NoRawat, "MedicationRequest", "", "skipped", "missing NIK")
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "skipped", "reason": "missing NIK"})
			failCount++
			continue
		}
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, "MedicationRequest", "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		practID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, "MedicationRequest", "", "failed", "practitioner lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		mr := buildMedReqJSON(row, patientID, practID, a.cfg.SSOrgID)
		fhirID, err := a.ss.SendMedicationRequest(mr)
		if err != nil {
			a.saveSendLog(row.NoRawat, "MedicationRequest", "", "failed", err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		if row.NoRacik == "" {
			_, dbErr := a.db.Exec("INSERT INTO satu_sehat_medicationrequest (no_resep, kode_brng, id_medicationrequest) VALUES (?,?,?)", row.NoResep, row.KodeBrng, fhirID)
			if dbErr != nil {
				log.Printf("⚠️ save med req %s: %v", fhirID, dbErr)
			}
		} else {
			_, dbErr := a.db.Exec("INSERT INTO satu_sehat_medicationrequest_racikan (no_resep, kode_brng, no_racik, id_medicationrequest) VALUES (?,?,?,?)", row.NoResep, row.KodeBrng, row.NoRacik, fhirID)
			if dbErr != nil {
				log.Printf("⚠️ save med req racikan %s: %v", fhirID, dbErr)
			}
		}
		a.saveSendLog(row.NoRawat, "MedicationRequest", fhirID, "success", "")
		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "obat": row.ObatDisplay,
			"status": "success", "fhir_id": fhirID,
		})
		sentCount++
	}
	jsonResponse(w, map[string]interface{}{"sent": sentCount, "failed": failCount, "details": results})
}
