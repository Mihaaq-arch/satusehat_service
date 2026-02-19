package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

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

// ============================================================
// FHIR RESOURCE SEND METHODS
// ============================================================

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

func (c *SSClient) SendProcedure(proc map[string]interface{}) (string, error) {
	result, err := c.doRequest("POST", "/Procedure", proc)
	if err != nil {
		return "", err
	}
	id, ok := result["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("procedure send failed: %v", result)
	}
	return id, nil
}

func (c *SSClient) SendMedicationRequest(mr map[string]interface{}) (string, error) {
	result, err := c.doRequest("POST", "/MedicationRequest", mr)
	if err != nil {
		return "", err
	}
	id, ok := result["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("medication request send failed: %v", result)
	}
	return id, nil
}

func (c *SSClient) SendMedicationDispense(md map[string]interface{}) (string, error) {
	result, err := c.doRequest("POST", "/MedicationDispense", md)
	if err != nil {
		return "", err
	}
	id, ok := result["id"].(string)
	if !ok || id == "" {
		return "", fmt.Errorf("medication dispense send failed: %v", result)
	}
	return id, nil
}
