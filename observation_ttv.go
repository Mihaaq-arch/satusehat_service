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
// OBSERVATION TTV (Tanda-Tanda Vital / Vital Signs)
// ============================================================

type TTVConfig struct {
	Name         string
	LOINCCode    string
	LOINCDisplay string
	Unit         string
	UnitCode     string
	DBColumn     string
	TrackTable   string
	IsComponent  bool
}

var ttvConfigs = []TTVConfig{
	{"suhu", "8310-5", "Body temperature", "degree Celsius", "Cel", "suhu_tubuh", "satu_sehat_observationttvsuhu", false},
	{"respirasi", "9279-1", "Respiratory rate", "breaths/minute", "/min", "respirasi", "satu_sehat_observationttvrespirasi", false},
	{"nadi", "8867-4", "Heart rate", "beats/minute", "/min", "nadi", "satu_sehat_observationttvnadi", false},
	{"spo2", "2708-6", "Oxygen saturation", "%", "%", "spo2", "satu_sehat_observationttvspo2", false},
	{"gcs", "9269-2", "Glasgow coma score total", "{score}", "{score}", "gcs", "satu_sehat_observationttvgcs", false},
	{"tensi", "35094-2", "Blood pressure panel", "mmHg", "mm[Hg]", "tensi", "satu_sehat_observationttvtensi", true},
	{"tb", "8302-2", "Body height", "centimeter", "cm", "tinggi", "satu_sehat_observationttvtb", false},
	{"bb", "29463-7", "Body Weight", "kilogram", "kg", "berat", "satu_sehat_observationttvbb", false},
	{"lp", "8280-0", "Waist Circumference at umbilicus by Tape measure", "centimeter", "cm", "lingkar_perut", "satu_sehat_observationttvlp", false},
}

type TTVRow struct {
	NoRawat       string
	NmPasien      string
	NoKTPPasien   string
	NoKTPDokter   string
	NamaDokter    string
	SttsLanjut    string
	IDEncounter   string
	TglPerawatan  string
	JamRawat      string
	Value         string
	IDObservation string
}

func queryPendingTTV(db *sql.DB, cfg TTVConfig, tgl1, tgl2 string) ([]TTVRow, error) {
	var results []TTVRow

	queryRalan := fmt.Sprintf(`
		SELECT reg_periksa.no_rawat, pasien.nm_pasien, pasien.no_ktp,
			pegawai.no_ktp as ktpdokter, pegawai.nama,
			'Ralan' as stts_lanjut,
			satu_sehat_encounter.id_encounter,
			pemeriksaan_ralan.tgl_perawatan, pemeriksaan_ralan.jam_rawat,
			pemeriksaan_ralan.%s,
			IFNULL(%s.id_observation,'') as id_observation
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN pemeriksaan_ralan ON pemeriksaan_ralan.no_rawat = reg_periksa.no_rawat
		INNER JOIN pegawai ON pemeriksaan_ralan.nip = pegawai.nik
		LEFT JOIN %s ON %s.no_rawat = pemeriksaan_ralan.no_rawat
			AND %s.tgl_perawatan = pemeriksaan_ralan.tgl_perawatan
			AND %s.jam_rawat = pemeriksaan_ralan.jam_rawat
			AND %s.status = 'Ralan'
		WHERE pemeriksaan_ralan.%s <> ''
			AND reg_periksa.tgl_registrasi BETWEEN ? AND ?`,
		cfg.DBColumn, cfg.TrackTable,
		cfg.TrackTable, cfg.TrackTable, cfg.TrackTable, cfg.TrackTable, cfg.TrackTable,
		cfg.DBColumn)

	rows, err := db.Query(queryRalan, tgl1, tgl2)
	if err != nil {
		return nil, fmt.Errorf("query ttv %s ralan: %w", cfg.Name, err)
	}
	for rows.Next() {
		var r TTVRow
		if err := rows.Scan(&r.NoRawat, &r.NmPasien, &r.NoKTPPasien,
			&r.NoKTPDokter, &r.NamaDokter, &r.SttsLanjut,
			&r.IDEncounter, &r.TglPerawatan, &r.JamRawat, &r.Value, &r.IDObservation); err != nil {
			log.Printf("⚠️ scan ttv %s ralan: %v", cfg.Name, err)
			continue
		}
		results = append(results, r)
	}
	rows.Close()

	queryRanap := fmt.Sprintf(`
		SELECT reg_periksa.no_rawat, pasien.nm_pasien, pasien.no_ktp,
			pegawai.no_ktp as ktpdokter, pegawai.nama,
			'Ranap' as stts_lanjut,
			satu_sehat_encounter.id_encounter,
			pemeriksaan_ranap.tgl_perawatan, pemeriksaan_ranap.jam_rawat,
			pemeriksaan_ranap.%s,
			IFNULL(%s.id_observation,'') as id_observation
		FROM reg_periksa
		INNER JOIN pasien ON reg_periksa.no_rkm_medis = pasien.no_rkm_medis
		INNER JOIN satu_sehat_encounter ON satu_sehat_encounter.no_rawat = reg_periksa.no_rawat
		INNER JOIN pemeriksaan_ranap ON pemeriksaan_ranap.no_rawat = reg_periksa.no_rawat
		INNER JOIN pegawai ON pemeriksaan_ranap.nip = pegawai.nik
		LEFT JOIN %s ON %s.no_rawat = pemeriksaan_ranap.no_rawat
			AND %s.tgl_perawatan = pemeriksaan_ranap.tgl_perawatan
			AND %s.jam_rawat = pemeriksaan_ranap.jam_rawat
			AND %s.status = 'Ranap'
		WHERE pemeriksaan_ranap.%s <> ''
			AND reg_periksa.tgl_registrasi BETWEEN ? AND ?`,
		cfg.DBColumn, cfg.TrackTable,
		cfg.TrackTable, cfg.TrackTable, cfg.TrackTable, cfg.TrackTable, cfg.TrackTable,
		cfg.DBColumn)

	rows2, err := db.Query(queryRanap, tgl1, tgl2)
	if err != nil {
		return results, fmt.Errorf("query ttv %s ranap: %w", cfg.Name, err)
	}
	defer rows2.Close()
	for rows2.Next() {
		var r TTVRow
		if err := rows2.Scan(&r.NoRawat, &r.NmPasien, &r.NoKTPPasien,
			&r.NoKTPDokter, &r.NamaDokter, &r.SttsLanjut,
			&r.IDEncounter, &r.TglPerawatan, &r.JamRawat, &r.Value, &r.IDObservation); err != nil {
			log.Printf("⚠️ scan ttv %s ranap: %v", cfg.Name, err)
			continue
		}
		results = append(results, r)
	}
	return results, nil
}

func buildObservationJSON(row TTVRow, cfg TTVConfig, patientID, practitionerID string) map[string]interface{} {
	effectiveDateTime := row.TglPerawatan + "T" + row.JamRawat + "+07:00"
	obs := map[string]interface{}{
		"resourceType": "Observation",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":    "vital-signs",
						"display": "Vital Signs",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://loinc.org",
					"code":    cfg.LOINCCode,
					"display": cfg.LOINCDisplay,
				},
			},
		},
		"subject":   map[string]interface{}{"reference": "Patient/" + patientID},
		"performer": []interface{}{map[string]interface{}{"reference": "Practitioner/" + practitionerID}},
		"encounter": map[string]interface{}{
			"reference": "Encounter/" + row.IDEncounter,
			"display":   "Pemeriksaan Fisik " + cfg.LOINCDisplay + ", Pasien " + row.NmPasien,
		},
		"effectiveDateTime": effectiveDateTime,
	}

	if cfg.IsComponent {
		parts := strings.Split(row.Value, "/")
		sistole, diastole := "0", "0"
		if len(parts) >= 1 && parts[0] != "" {
			sistole = strings.ReplaceAll(parts[0], ",", ".")
		}
		if len(parts) >= 2 && parts[1] != "" {
			diastole = strings.ReplaceAll(parts[1], ",", ".")
		}
		obs["component"] = []interface{}{
			map[string]interface{}{
				"code": map[string]interface{}{
					"coding": []interface{}{map[string]interface{}{"system": "http://loinc.org", "code": "8480-6", "display": "Systolic blood pressure"}},
				},
				"valueQuantity": map[string]interface{}{"value": parseFloat(sistole), "unit": "mmHg", "system": "http://unitsofmeasure.org", "code": "mm[Hg]"},
			},
			map[string]interface{}{
				"code": map[string]interface{}{
					"coding": []interface{}{map[string]interface{}{"system": "http://loinc.org", "code": "8462-4", "display": "Diastolic blood pressure"}},
				},
				"valueQuantity": map[string]interface{}{"value": parseFloat(diastole), "unit": "mmHg", "system": "http://unitsofmeasure.org", "code": "mm[Hg]"},
			},
		}
	} else {
		valStr := strings.ReplaceAll(row.Value, ",", ".")
		obs["valueQuantity"] = map[string]interface{}{
			"value": parseFloat(valStr), "unit": cfg.Unit, "system": "http://unitsofmeasure.org", "code": cfg.UnitCode,
		}
	}

	return obs
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func findTTVConfig(name string) *TTVConfig {
	for i := range ttvConfigs {
		if ttvConfigs[i].Name == name {
			return &ttvConfigs[i]
		}
	}
	return nil
}

func (a *App) handlePendingTTV(w http.ResponseWriter, r *http.Request) {
	ttvType := r.PathValue("type")
	cfg := findTTVConfig(ttvType)
	if cfg == nil {
		jsonError(w, "unknown TTV type: "+ttvType+". Valid: suhu,respirasi,nadi,spo2,gcs,tensi,tb,bb,lp", 400)
		return
	}
	tgl1 := r.URL.Query().Get("tgl1")
	tgl2 := r.URL.Query().Get("tgl2")
	if tgl1 == "" || tgl2 == "" {
		today := time.Now().Format("2006-01-02")
		tgl1, tgl2 = today, today
	}
	rows, err := queryPendingTTV(a.db, *cfg, tgl1, tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var pending, sent []TTVRow
	for _, row := range rows {
		if row.IDObservation == "" {
			pending = append(pending, row)
		} else {
			sent = append(sent, row)
		}
	}
	jsonResponse(w, map[string]interface{}{
		"type": ttvType, "tgl1": tgl1, "tgl2": tgl2,
		"total": len(rows), "pending_count": len(pending), "sent_count": len(sent),
		"pending": pending,
	})
}

func (a *App) handleSendTTV(w http.ResponseWriter, r *http.Request) {
	ttvType := r.PathValue("type")
	cfg := findTTVConfig(ttvType)
	if cfg == nil {
		jsonError(w, "unknown TTV type: "+ttvType, 400)
		return
	}
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
	rows, err := queryPendingTTV(a.db, *cfg, req.Tgl1, req.Tgl2)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var results []map[string]interface{}
	sentCount, failCount := 0, 0
	resourceLabel := "Observation_" + cfg.Name
	for _, row := range rows {
		if row.IDObservation != "" {
			continue
		}
		if row.NoKTPPasien == "" || row.NoKTPDokter == "" {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "skipped", "missing NIK")
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "status": "skipped", "reason": "missing NIK"})
			failCount++
			continue
		}
		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "status": "failed", "error": "patient lookup: " + err.Error()})
			failCount++
			continue
		}
		practitionerID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "failed", "practitioner lookup: "+err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "status": "failed", "error": "practitioner lookup: " + err.Error()})
			failCount++
			continue
		}
		obs := buildObservationJSON(row, *cfg, patientID, practitionerID)
		fhirID, err := a.sendViaJob("Observation_"+cfg.Name, idempKey(row.NoRawat, row.TglPerawatan, row.JamRawat, row.SttsLanjut), obs, a.ss.SendObservation)
		if err != nil {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "failed", err.Error())
			results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "status": "failed", "error": err.Error()})
			failCount++
			continue
		}
		if fhirID == "" {
			continue
		}
		_, dbErr := a.db.Exec(
			fmt.Sprintf("INSERT INTO %s (no_rawat, tgl_perawatan, jam_rawat, status, id_observation) VALUES (?,?,?,?,?)", cfg.TrackTable),
			row.NoRawat, row.TglPerawatan, row.JamRawat, row.SttsLanjut, fhirID)
		if dbErr != nil {
			log.Printf("⚠️ save observation %s to %s: %v", fhirID, cfg.TrackTable, dbErr)
		}
		a.saveSendLog(row.NoRawat, resourceLabel, fhirID, "success", "")
		results = append(results, map[string]interface{}{"no_rawat": row.NoRawat, "status": "success", "fhir_id": fhirID})
		sentCount++
	}
	jsonResponse(w, map[string]interface{}{
		"type": ttvType, "sent": sentCount, "failed": failCount, "details": results,
	})
}
