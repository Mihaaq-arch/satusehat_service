package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================================
// MEDICATION DISPENSE
// ============================================================

type MedDispRow struct {
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
	IDMedDisp    string
	NoBatch      string
	NoFaktur     string
	TglValidasi  string
	SttsLanjut   string
	IDLocation   string
	NmBangsal    string
}

func queryPendingMedDisp(db *sql.DB, tgl1, tgl2 string) ([]MedDispRow, error) {
	query := `
		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			pegawai.nama, pegawai.no_ktp as ktppraktisi, satu_sehat_encounter.id_encounter,
			satu_sehat_mapping_obat.obat_code, satu_sehat_mapping_obat.obat_system,
			detail_pemberian_obat.kode_brng, satu_sehat_mapping_obat.obat_display,
			satu_sehat_mapping_obat.form_code, satu_sehat_mapping_obat.form_system, satu_sehat_mapping_obat.form_display,
			satu_sehat_mapping_obat.route_code, satu_sehat_mapping_obat.route_system, satu_sehat_mapping_obat.route_display,
			satu_sehat_mapping_obat.denominator_code, satu_sehat_mapping_obat.denominator_system,
			CONCAT(resep_obat.tgl_peresepan,' ',resep_obat.jam_peresepan) as tgl_peresepan,
			detail_pemberian_obat.jml, satu_sehat_medication.id_medication,
			aturan_pakai.aturan, resep_obat.no_resep,
			IFNULL(satu_sehat_medicationdispense.id_medicationdispanse,'') as id_medicationdispanse,
			detail_pemberian_obat.no_batch, detail_pemberian_obat.no_faktur,
			CONCAT(detail_pemberian_obat.tgl_perawatan,' ',detail_pemberian_obat.jam) as tgl_validasi,
			'Ralan' as stts_lanjut,
			satu_sehat_mapping_lokasi_depo_farmasi.id_lokasi_satusehat, bangsal.nm_bangsal
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN resep_obat ON reg_periksa.no_rawat = resep_obat.no_rawat
		INNER JOIN pegawai ON resep_obat.kd_dokter = pegawai.nik
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN detail_pemberian_obat ON detail_pemberian_obat.no_rawat = resep_obat.no_rawat
			AND detail_pemberian_obat.tgl_perawatan = resep_obat.tgl_perawatan
			AND detail_pemberian_obat.jam = resep_obat.jam
		INNER JOIN aturan_pakai ON detail_pemberian_obat.no_rawat = aturan_pakai.no_rawat
			AND detail_pemberian_obat.tgl_perawatan = aturan_pakai.tgl_perawatan
			AND detail_pemberian_obat.jam = aturan_pakai.jam
			AND detail_pemberian_obat.kode_brng = aturan_pakai.kode_brng
		INNER JOIN satu_sehat_mapping_obat ON satu_sehat_mapping_obat.kode_brng = detail_pemberian_obat.kode_brng
		INNER JOIN bangsal ON bangsal.kd_bangsal = detail_pemberian_obat.kd_bangsal
		INNER JOIN satu_sehat_mapping_lokasi_depo_farmasi ON satu_sehat_mapping_lokasi_depo_farmasi.kd_bangsal = bangsal.kd_bangsal
		INNER JOIN satu_sehat_medication ON satu_sehat_medication.kode_brng = satu_sehat_mapping_obat.kode_brng
		LEFT JOIN satu_sehat_medicationdispense ON satu_sehat_medicationdispense.no_rawat = detail_pemberian_obat.no_rawat
			AND satu_sehat_medicationdispense.tgl_perawatan = detail_pemberian_obat.tgl_perawatan
			AND satu_sehat_medicationdispense.jam = detail_pemberian_obat.jam
			AND satu_sehat_medicationdispense.kode_brng = detail_pemberian_obat.kode_brng
			AND satu_sehat_medicationdispense.no_batch = detail_pemberian_obat.no_batch
			AND satu_sehat_medicationdispense.no_faktur = detail_pemberian_obat.no_faktur
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
		  AND reg_periksa.status_lanjut = 'Ralan'

		UNION ALL

		SELECT reg_periksa.no_rawat, reg_periksa.no_rkm_medis, pasien.nm_pasien, pasien.no_ktp,
			pegawai.nama, pegawai.no_ktp as ktppraktisi, satu_sehat_encounter.id_encounter,
			satu_sehat_mapping_obat.obat_code, satu_sehat_mapping_obat.obat_system,
			detail_pemberian_obat.kode_brng, satu_sehat_mapping_obat.obat_display,
			satu_sehat_mapping_obat.form_code, satu_sehat_mapping_obat.form_system, satu_sehat_mapping_obat.form_display,
			satu_sehat_mapping_obat.route_code, satu_sehat_mapping_obat.route_system, satu_sehat_mapping_obat.route_display,
			satu_sehat_mapping_obat.denominator_code, satu_sehat_mapping_obat.denominator_system,
			CONCAT(resep_obat.tgl_peresepan,' ',resep_obat.jam_peresepan) as tgl_peresepan,
			detail_pemberian_obat.jml, satu_sehat_medication.id_medication,
			aturan_pakai.aturan, resep_obat.no_resep,
			IFNULL(satu_sehat_medicationdispense.id_medicationdispanse,'') as id_medicationdispanse,
			detail_pemberian_obat.no_batch, detail_pemberian_obat.no_faktur,
			CONCAT(detail_pemberian_obat.tgl_perawatan,' ',detail_pemberian_obat.jam) as tgl_validasi,
			'Ranap' as stts_lanjut,
			satu_sehat_mapping_lokasi_depo_farmasi.id_lokasi_satusehat, bangsal.nm_bangsal
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN resep_obat ON reg_periksa.no_rawat = resep_obat.no_rawat
		INNER JOIN pegawai ON resep_obat.kd_dokter = pegawai.nik
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN detail_pemberian_obat ON detail_pemberian_obat.no_rawat = resep_obat.no_rawat
			AND detail_pemberian_obat.tgl_perawatan = resep_obat.tgl_perawatan
			AND detail_pemberian_obat.jam = resep_obat.jam
		INNER JOIN aturan_pakai ON detail_pemberian_obat.no_rawat = aturan_pakai.no_rawat
			AND detail_pemberian_obat.tgl_perawatan = aturan_pakai.tgl_perawatan
			AND detail_pemberian_obat.jam = aturan_pakai.jam
			AND detail_pemberian_obat.kode_brng = aturan_pakai.kode_brng
		INNER JOIN satu_sehat_mapping_obat ON satu_sehat_mapping_obat.kode_brng = detail_pemberian_obat.kode_brng
		INNER JOIN bangsal ON bangsal.kd_bangsal = detail_pemberian_obat.kd_bangsal
		INNER JOIN satu_sehat_mapping_lokasi_depo_farmasi ON satu_sehat_mapping_lokasi_depo_farmasi.kd_bangsal = bangsal.kd_bangsal
		INNER JOIN satu_sehat_medication ON satu_sehat_medication.kode_brng = satu_sehat_mapping_obat.kode_brng
		LEFT JOIN satu_sehat_medicationdispense ON satu_sehat_medicationdispense.no_rawat = detail_pemberian_obat.no_rawat
			AND satu_sehat_medicationdispense.tgl_perawatan = detail_pemberian_obat.tgl_perawatan
			AND satu_sehat_medicationdispense.jam = detail_pemberian_obat.jam
			AND satu_sehat_medicationdispense.kode_brng = detail_pemberian_obat.kode_brng
			AND satu_sehat_medicationdispense.no_batch = detail_pemberian_obat.no_batch
			AND satu_sehat_medicationdispense.no_faktur = detail_pemberian_obat.no_faktur
		WHERE reg_periksa.tgl_registrasi BETWEEN ? AND ?
		  AND reg_periksa.status_lanjut = 'Ranap'`

	rows, err := db.Query(query, tgl1, tgl2, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query medication dispense: %w", err)
	}
	defer rows.Close()

	var results []MedDispRow
	for rows.Next() {
		var r MedDispRow
		if err := rows.Scan(&r.NoRawat, &r.NoRM, &r.NmPasien, &r.NoKTPPasien,
			&r.NmDokter, &r.NoKTPDokter, &r.IDEncounter,
			&r.ObatCode, &r.ObatSystem, &r.KodeBrng, &r.ObatDisplay,
			&r.FormCode, &r.FormSystem, &r.FormDisplay,
			&r.RouteCode, &r.RouteSystem, &r.RouteDisplay,
			&r.DenomCode, &r.DenomSystem,
			&r.TglPeresepan, &r.Jml, &r.IDMedication,
			&r.AturanPakai, &r.NoResep, &r.IDMedDisp,
			&r.NoBatch, &r.NoFaktur, &r.TglValidasi,
			&r.SttsLanjut, &r.IDLocation, &r.NmBangsal); err != nil {
			log.Printf("⚠️ scan med disp: %v", err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func lookupMedReqID(db *sql.DB, noResep, kodeBrng string) string {
	var id string
	err := db.QueryRow(
		"SELECT id_medicationrequest FROM satu_sehat_medicationrequest WHERE no_resep=? AND kode_brng=?",
		noResep, kodeBrng).Scan(&id)
	if err != nil {
		return ""
	}
	return id
}

func buildMedDispJSON(row MedDispRow, patientID, practitionerID, orgID, medReqID string) map[string]interface{} {
	signa1, signa2 := parseSigna(row.AturanPakai)
	signa1f, _ := strconv.ParseFloat(signa1, 64)
	signa2f, _ := strconv.ParseFloat(signa2, 64)
	jmlf, _ := strconv.ParseFloat(row.Jml, 64)

	catCode, catDisplay := "outpatient", "Outpatient"
	if row.SttsLanjut == "Ranap" {
		catCode, catDisplay = "inpatient", "Inpatient"
	}

	whenPrepared := strings.ReplaceAll(row.TglPeresepan, " ", "T") + "+07:00"
	whenHandedOver := strings.ReplaceAll(row.TglValidasi, " ", "T") + "+07:00"

	md := map[string]interface{}{
		"resourceType": "MedicationDispense",
		"identifier": []interface{}{
			map[string]interface{}{"system": "http://sys-ids.kemkes.go.id/medicationdispense/" + orgID, "use": "official", "value": row.NoResep},
			map[string]interface{}{"system": "http://sys-ids.kemkes.go.id/medicationdispense-item/" + orgID, "use": "official", "value": row.KodeBrng},
		},
		"status": "completed",
		"category": map[string]interface{}{
			"coding": []interface{}{map[string]interface{}{"system": "http://terminology.hl7.org/fhir/CodeSystem/medicationdispense-category", "code": catCode, "display": catDisplay}},
		},
		"medicationReference": map[string]interface{}{"reference": "Medication/" + row.IDMedication, "display": row.ObatDisplay},
		"subject":             map[string]interface{}{"reference": "Patient/" + patientID, "display": row.NmPasien},
		"context":             map[string]interface{}{"reference": "Encounter/" + row.IDEncounter},
		"performer": []interface{}{
			map[string]interface{}{"actor": map[string]interface{}{"reference": "Practitioner/" + practitionerID, "display": row.NmDokter}},
		},
		"location":       map[string]interface{}{"reference": "Location/" + row.IDLocation, "display": row.NmBangsal},
		"quantity":       map[string]interface{}{"system": row.DenomSystem, "code": row.DenomCode, "value": jmlf},
		"whenPrepared":   whenPrepared,
		"whenHandedOver": whenHandedOver,
		"dosageInstruction": []interface{}{
			map[string]interface{}{
				"sequence": 1, "text": row.AturanPakai,
				"timing": map[string]interface{}{"repeat": map[string]interface{}{"frequency": signa2f, "period": 1, "periodUnit": "d"}},
				"route":  map[string]interface{}{"coding": []interface{}{map[string]interface{}{"system": row.RouteSystem, "code": row.RouteCode, "display": row.RouteDisplay}}},
				"doseAndRate": []interface{}{
					map[string]interface{}{"doseQuantity": map[string]interface{}{"value": signa1f, "unit": row.DenomCode, "system": row.DenomSystem, "code": row.DenomCode}},
				},
			},
		},
	}
	if medReqID != "" {
		md["authorizingPrescription"] = []interface{}{map[string]interface{}{"reference": "MedicationRequest/" + medReqID}}
	}
	return md
}

// ============================================================
// MEDICATION DISPENSE HANDLERS
// ============================================================

func (a *App) handlePendingMedDisp(w http.ResponseWriter, r *http.Request) {
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1, tgl2 = today, today
	}
	rows, err := queryPendingMedDisp(a.db, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var pending, sent []MedDispRow
	for _, row := range rows {
		if row.IDMedDisp == "" {
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

func (a *App) handleSendMedDisp(w http.ResponseWriter, r *http.Request) {
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
	rows, err := queryPendingMedDisp(a.db, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var results []map[string]interface{}
	sentCount, failCount := 0, 0
	for _, row := range rows {
		if row.IDMedDisp != "" {
			continue
		}
		if row.NoKTPPasien == "" || row.NoKTPDokter == "" {
			a.saveSendLog(row.NoRawat, "MedicationDispense", "", "skipped", "missing NIK")
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "skipped", "reason": "missing NIK"})
			failCount++
			continue
		}
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, "MedicationDispense", "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		practID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, "MedicationDispense", "", "failed", "practitioner lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		medReqID := lookupMedReqID(a.db, row.NoResep, row.KodeBrng)
		md := buildMedDispJSON(row, patientID, practID, a.cfg.SSOrgID, medReqID)
		fhirID, err := a.sendViaJob("MedicationDispense", idempKey(row.NoRawat, row.TglValidasi, row.KodeBrng, row.NoBatch, row.NoFaktur), md, a.ss.SendMedicationDispense)
		if err != nil {
			a.saveSendLog(row.NoRawat, "MedicationDispense", "", "failed", err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		if fhirID == "" {
			continue
		}
		tglParts := strings.SplitN(row.TglValidasi, " ", 2)
		tglPerawatan := tglParts[0]
		jam := ""
		if len(tglParts) > 1 {
			jam = tglParts[1]
		}
		_, dbErr := a.db.Exec(
			"INSERT INTO satu_sehat_medicationdispense (no_rawat, tgl_perawatan, jam, kode_brng, no_batch, no_faktur, id_medicationdispanse) VALUES (?,?,?,?,?,?,?)",
			row.NoRawat, tglPerawatan, jam, row.KodeBrng, row.NoBatch, row.NoFaktur, fhirID)
		if dbErr != nil {
			log.Printf("⚠️ save med disp %s: %v", fhirID, dbErr)
		}
		a.saveSendLog(row.NoRawat, "MedicationDispense", fhirID, "success", "")
		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "kode_brng": row.KodeBrng, "obat": row.ObatDisplay,
			"status": "success", "fhir_id": fhirID,
		})
		sentCount++
	}
	jsonResponse(w, map[string]interface{}{"sent": sentCount, "failed": failCount, "details": results})
}
