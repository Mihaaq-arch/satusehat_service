package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
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
// TOKEN MANAGER (OAuth2 with auto-refresh)
// ============================================================

type TokenManager struct {
	cfg       Config
	token     string
	expiresAt time.Time
	mu        sync.RWMutex
}

func NewTokenManager(cfg Config) *TokenManager {
	return &TokenManager{cfg: cfg}
}

func (tm *TokenManager) GetToken() (string, error) {
	tm.mu.RLock()
	if tm.token != "" && time.Now().Before(tm.expiresAt) {
		defer tm.mu.RUnlock()
		return tm.token, nil
	}
	tm.mu.RUnlock()

	// Need to refresh
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Double-check after acquiring write lock
	if tm.token != "" && time.Now().Before(tm.expiresAt) {
		return tm.token, nil
	}

	data := url.Values{}
	data.Set("client_id", tm.cfg.SSClientID)
	data.Set("client_secret", tm.cfg.SSSecret)

	resp, err := http.PostForm(tm.cfg.SSAuthURL+"/accesstoken?grant_type=client_credentials", data)
	if err != nil {
		return "", fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("token error %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}

	tm.token = result.AccessToken
	expiresIn, _ := strconv.Atoi(result.ExpiresIn)
	tm.expiresAt = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
	log.Printf("✅ Token refreshed, expires in %ss", result.ExpiresIn)
	return tm.token, nil
}

// ============================================================
// SATU SEHAT CLIENT
// ============================================================

type SSClient struct {
	cfg      Config
	tokenMgr *TokenManager
	http     *http.Client
}

func NewSSClient(cfg Config, tm *TokenManager) *SSClient {
	return &SSClient{
		cfg:      cfg,
		tokenMgr: tm,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

// doRequest makes an authenticated FHIR request
func (c *SSClient) doRequest(method, path string, body interface{}) (map[string]interface{}, error) {
	token, err := c.tokenMgr.GetToken()
	if err != nil {
		return nil, err
	}

	var reqBody io.Reader
	if body != nil {
		jsonBytes, _ := json.Marshal(body)
		reqBody = bytes.NewReader(jsonBytes)
		log.Printf("📤 %s %s\n%s", method, path, string(jsonBytes))
	}

	req, err := http.NewRequest(method, c.cfg.SSFHIRURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("📥 Response %d:\n%s", resp.StatusCode, string(respBody))

	var result map[string]interface{}
	json.Unmarshal(respBody, &result)
	return result, nil
}

// LookupPatient looks up a FHIR Patient ID by NIK
func (c *SSClient) LookupPatient(nik string) (string, error) {
	result, err := c.doRequest("GET", "/Patient?identifier=https://fhir.kemkes.go.id/id/nik|"+nik, nil)
	if err != nil {
		return "", err
	}

	// Parse FHIR Bundle response
	total, _ := result["total"].(float64)
	if total == 0 {
		return "", fmt.Errorf("patient NIK %s not found", nik)
	}

	entries, ok := result["entry"].([]interface{})
	if !ok || len(entries) == 0 {
		return "", fmt.Errorf("patient NIK %s: no entries", nik)
	}

	entry := entries[0].(map[string]interface{})
	resource := entry["resource"].(map[string]interface{})
	id := resource["id"].(string)
	return id, nil
}

// LookupPractitioner looks up a FHIR Practitioner ID by NIK
func (c *SSClient) LookupPractitioner(nik string) (string, error) {
	result, err := c.doRequest("GET", "/Practitioner?identifier=https://fhir.kemkes.go.id/id/nik|"+nik, nil)
	if err != nil {
		return "", err
	}

	total, _ := result["total"].(float64)
	if total == 0 {
		return "", fmt.Errorf("practitioner NIK %s not found", nik)
	}

	entries, ok := result["entry"].([]interface{})
	if !ok || len(entries) == 0 {
		return "", fmt.Errorf("practitioner NIK %s: no entries", nik)
	}

	entry := entries[0].(map[string]interface{})
	resource := entry["resource"].(map[string]interface{})
	id := resource["id"].(string)
	return id, nil
}

// SendEncounter sends encounter FHIR resource
func (c *SSClient) SendEncounter(enc map[string]interface{}) (string, error) {
	result, err := c.doRequest("POST", "/Encounter", enc)
	if err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("encounter send failed: %v", result)
	}
	return id, nil
}

// SendCondition sends condition FHIR resource
func (c *SSClient) SendCondition(cond map[string]interface{}) (string, error) {
	result, err := c.doRequest("POST", "/Condition", cond)
	if err != nil {
		return "", err
	}

	id, ok := result["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("condition send failed: %v", result)
	}
	return id, nil
}

// ============================================================
// ENCOUNTER ROW (from DB query)
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

// ============================================================
// CONDITION ROW (from DB query)
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

// ============================================================
// DATABASE QUERIES
// ============================================================

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

// ============================================================
// FHIR RESOURCE BUILDERS
// ============================================================

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
// OBSERVATION TTV (Tanda-Tanda Vital / Vital Signs)
// ============================================================

// TTVConfig defines one vital sign type with its LOINC code, unit, DB column, and tracking table
type TTVConfig struct {
	Name         string // e.g. "suhu", "nadi", "tensi"
	LOINCCode    string
	LOINCDisplay string
	Unit         string // UCUM unit display
	UnitCode     string // UCUM code
	DBColumn     string // column in pemeriksaan_ralan/ranap
	TrackTable   string // satu_sehat_observationttvXXX
	IsComponent  bool   // true for tensi (blood pressure uses component)
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
	SttsLanjut    string // "Ralan" or "Ranap"
	IDEncounter   string
	TglPerawatan  string
	JamRawat      string
	Value         string // the vital sign value
	IDObservation string // empty if not sent
}

func queryPendingTTV(db *sql.DB, cfg TTVConfig, tgl1, tgl2 string) ([]TTVRow, error) {
	var results []TTVRow

	// Query ralan
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

	// Query ranap
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
		"subject": map[string]interface{}{
			"reference": "Patient/" + patientID,
		},
		"performer": []interface{}{
			map[string]interface{}{
				"reference": "Practitioner/" + practitionerID,
			},
		},
		"encounter": map[string]interface{}{
			"reference": "Encounter/" + row.IDEncounter,
			"display":   "Pemeriksaan Fisik " + cfg.LOINCDisplay + ", Pasien " + row.NmPasien,
		},
		"effectiveDateTime": effectiveDateTime,
	}

	if cfg.IsComponent {
		// Blood pressure: split "120/80" into systolic/diastolic components
		parts := strings.Split(row.Value, "/")
		sistole := "0"
		diastole := "0"
		if len(parts) >= 1 && parts[0] != "" {
			sistole = strings.ReplaceAll(parts[0], ",", ".")
		}
		if len(parts) >= 2 && parts[1] != "" {
			diastole = strings.ReplaceAll(parts[1], ",", ".")
		}
		obs["component"] = []interface{}{
			map[string]interface{}{
				"code": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{
							"system": "http://loinc.org", "code": "8480-6", "display": "Systolic blood pressure",
						},
					},
				},
				"valueQuantity": map[string]interface{}{
					"value": parseFloat(sistole), "unit": "mmHg",
					"system": "http://unitsofmeasure.org", "code": "mm[Hg]",
				},
			},
			map[string]interface{}{
				"code": map[string]interface{}{
					"coding": []interface{}{
						map[string]interface{}{
							"system": "http://loinc.org", "code": "8462-4", "display": "Diastolic blood pressure",
						},
					},
				},
				"valueQuantity": map[string]interface{}{
					"value": parseFloat(diastole), "unit": "mmHg",
					"system": "http://unitsofmeasure.org", "code": "mm[Hg]",
				},
			},
		}
	} else {
		valStr := strings.ReplaceAll(row.Value, ",", ".")
		obs["valueQuantity"] = map[string]interface{}{
			"value":  parseFloat(valStr),
			"unit":   cfg.Unit,
			"system": "http://unitsofmeasure.org",
			"code":   cfg.UnitCode,
		}
	}

	return obs
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

// SendObservation sends observation FHIR resource
func (c *SSClient) SendObservation(obs map[string]interface{}) (string, error) {
	result, err := c.doRequest("POST", "/Observation", obs)
	if err != nil {
		return "", err
	}
	id, ok := result["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("observation send failed: %v", result)
	}
	return id, nil
}

// ============================================================
// HTTP HANDLERS
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
	// Quick DB check
	err := a.db.Ping()
	dbStatus := "ok"
	if err != nil {
		dbStatus = err.Error()
	}

	// Quick token check
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
// TTV OBSERVATION HANDLERS
// ============================================================

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
		tgl1 = today
		tgl2 = today
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
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "skipped", "reason": "missing NIK",
			})
			failCount++
			continue
		}

		patientID, err := a.ss.LookupPatient(row.NoKTPPasien)
		if err != nil {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "failed", "patient lookup: "+err.Error())
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "failed", "error": "patient lookup: " + err.Error(),
			})
			failCount++
			continue
		}

		practitionerID, err := a.ss.LookupPractitioner(row.NoKTPDokter)
		if err != nil {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "failed", "practitioner lookup: "+err.Error())
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "failed", "error": "practitioner lookup: " + err.Error(),
			})
			failCount++
			continue
		}

		obs := buildObservationJSON(row, *cfg, patientID, practitionerID)
		fhirID, err := a.ss.SendObservation(obs)
		if err != nil {
			a.saveSendLog(row.NoRawat, resourceLabel, "", "failed", err.Error())
			results = append(results, map[string]interface{}{
				"no_rawat": row.NoRawat, "status": "failed", "error": err.Error(),
			})
			failCount++
			continue
		}

		// Save to the appropriate tracking table (same schema as Khanza)
		_, dbErr := a.db.Exec(
			fmt.Sprintf("INSERT INTO %s (no_rawat, tgl_perawatan, jam_rawat, status, id_observation) VALUES (?,?,?,?,?)", cfg.TrackTable),
			row.NoRawat, row.TglPerawatan, row.JamRawat, row.SttsLanjut, fhirID)
		if dbErr != nil {
			log.Printf("⚠️ save observation %s to %s: %v", fhirID, cfg.TrackTable, dbErr)
		}

		a.saveSendLog(row.NoRawat, resourceLabel, fhirID, "success", "")
		results = append(results, map[string]interface{}{
			"no_rawat": row.NoRawat, "status": "success", "fhir_id": fhirID,
		})
		sentCount++
	}

	jsonResponse(w, map[string]interface{}{
		"type": ttvType, "sent": sentCount, "failed": failCount, "details": results,
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

	// Init token manager and SS client
	tokenMgr := NewTokenManager(cfg)
	ssClient := NewSSClient(cfg, tokenMgr)

	app := &App{db: db, ss: ssClient, cfg: cfg}

	// Routes
	mux := http.NewServeMux()
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

// Trick to suppress "unused import" for strings package
var _ = strings.TrimSpace
