# Satu Sehat Go Service

Microservice penghubung SIMRS Khanza dengan **Satu Sehat** FHIR R4 API (Kemenkes RI).  
Dibangun dengan Go — tanpa framework, satu binary executable.

## Fitur

| Resource | Endpoint | Keterangan |
|----------|----------|------------|
| **Encounter Ralan** | `GET /api/encounters/pending` | List encounter rawat jalan yang belum dikirim |
| | `POST /api/encounters/send` | Kirim encounter ralan ke Satu Sehat |
| **Encounter Ranap** | `GET /api/encounters-ranap/pending` | List encounter rawat inap |
| | `POST /api/encounters-ranap/send` | Kirim encounter ranap ke Satu Sehat |
| **Condition** | `GET /api/conditions/pending` | List diagnosa (ICD-10) yang belum dikirim |
| | `POST /api/conditions/send` | Kirim diagnosa ke Satu Sehat |
| **Observation TTV** | `GET /api/observations-ttv/{type}/pending` | List vital signs per tipe |
| | `POST /api/observations-ttv/{type}/send` | Kirim vital signs ke Satu Sehat |
| **Send Log** | `GET /api/logs` | Riwayat pengiriman (filter by tanggal & status) |
| **Health** | `GET /api/health` | Status koneksi DB & token |

### Tipe TTV yang Didukung

`suhu` · `respirasi` · `nadi` · `spo2` · `gcs` · `tensi` · `tb` · `bb` · `lp`

> Tensi otomatis split "120/80" → systolic + diastolic FHIR component.

## Arsitektur

```
                     ┌─────────────────┐
                     │   Satu Sehat    │
                     │  FHIR R4 API    │
                     └────────▲────────┘
                              │ HTTPS
┌──────────┐   HTTP    ┌──────┴──────┐   MySQL   ┌──────────┐
│  Client  │◄─────────►│  Go Service │◄──────────►│  Khanza  │
│ (React/  │  :8089    │  (main.go)  │  :3939    │    DB    │
│  Postman)│           └─────────────┘           │  (sik)   │
└──────────┘                                     └──────────┘
```

## Quick Start

### 1. Konfigurasi

Buat file `.env` di root project:

```env
# Database (sama dengan Khanza)
DB_HOST=localhost
DB_PORT=3306
DB_USER=your_user
DB_PASS=your_password
DB_NAME=sik

# Satu Sehat API
SS_CLIENT_ID=your_client_id
SS_CLIENT_SECRET=your_client_secret
SS_AUTH_URL=https://api-satusehat-dev.dto.kemkes.go.id/oauth2/v1
SS_FHIR_URL=https://api-satusehat-dev.dto.kemkes.go.id/fhir-r4/v1
SS_ORG_ID=your_org_id

# Server
PORT=8089
```

### 2. Build & Run

```bash
# Build
go build -o satusehat-service.exe .

# Run (langsung)
go run .

# Atau run binary
./satusehat-service.exe
```

### 3. Test

```bash
# Health check
curl http://localhost:8089/api/health

# List pending encounter ralan hari ini
curl http://localhost:8089/api/encounters/pending

# List pending encounter dengan rentang tanggal
curl "http://localhost:8089/api/encounters/pending?tgl1=2026-02-01&tgl2=2026-02-18"

# List pending TTV suhu
curl "http://localhost:8089/api/observations-ttv/suhu/pending?tgl1=2026-02-01&tgl2=2026-02-18"

# Kirim encounter ralan
curl -X POST http://localhost:8089/api/encounters/send \
  -H "Content-Type: application/json" \
  -d '{"tgl1":"2026-02-01","tgl2":"2026-02-18"}'

# Kirim TTV tensi
curl -X POST http://localhost:8089/api/observations-ttv/tensi/send \
  -H "Content-Type: application/json" \
  -d '{"tgl1":"2026-02-01","tgl2":"2026-02-18"}'

# Cek log pengiriman
curl "http://localhost:8089/api/logs?status=failed&limit=20"
```

## Tabel Database

Service ini menggunakan tabel-tabel Khanza yang sudah ada dan otomatis membuat:

| Tabel | Keterangan |
|-------|------------|
| `satu_sehat_encounter` | Tracking encounter yang sudah dikirim |
| `satu_sehat_condition` | Tracking diagnosa yang sudah dikirim |
| `satu_sehat_observationttvsuhu` | Tracking TTV suhu |
| `satu_sehat_observationttvrespirasi` | Tracking TTV respirasi |
| `satu_sehat_observationttvnadi` | Tracking TTV nadi |
| `satu_sehat_observationttvspo2` | Tracking TTV SpO2 |
| `satu_sehat_observationttvgcs` | Tracking TTV GCS |
| `satu_sehat_observationttvtensi` | Tracking TTV tensi |
| `satu_sehat_observationttvtb` | Tracking TTV tinggi badan |
| `satu_sehat_observationttvbb` | Tracking TTV berat badan |
| `satu_sehat_observationttvlp` | Tracking TTV lingkar perut |
| `satu_sehat_send_log` | **Auto-create.** Log semua pengiriman |

## Environment

| Variable | Deskripsi | Contoh |
|----------|-----------|--------|
| `DB_HOST` | MySQL host | `localhost` |
| `DB_PORT` | MySQL port | `3306` |
| `DB_USER` | MySQL user | `root` |
| `DB_PASS` | MySQL password | `***` |
| `DB_NAME` | Database name | `sik` |
| `SS_CLIENT_ID` | Satu Sehat Client ID | dari Kemenkes |
| `SS_CLIENT_SECRET` | Satu Sehat Secret | dari Kemenkes |
| `SS_AUTH_URL` | OAuth2 endpoint | `.../oauth2/v1` |
| `SS_FHIR_URL` | FHIR R4 endpoint | `.../fhir-r4/v1` |
| `SS_ORG_ID` | Organization ID | dari Kemenkes |
| `PORT` | HTTP port | `8089` |

## Perbedaan dengan Java (Khanza)

| Aspek | Java (Khanza) | Go Service |
|-------|--------------|------------|
| Token | Request setiap kali kirim | Cache + auto-refresh |
| TTV | 10x copy-paste (~3000 baris) | Data-driven config (~200 baris) |
| Delivery | Desktop GUI (Swing) | REST API → bisa diakses dari mana saja |
| Encounter | Hanya ralan | Ralan + Ranap |
| Logging | Print ke console | Simpan ke DB (`satu_sehat_send_log`) |

## Roadmap

- [x] Encounter Ralan
- [x] Encounter Ranap
- [x] Condition (Diagnosa ICD-10)
- [x] Observation TTV (9 tipe)
- [x] Send Log & Log Endpoint
- [ ] Observation Lab
- [ ] Observation Radiologi  
- [ ] Procedure
- [ ] MedicationRequest / MedicationDispense
- [ ] Background retry worker
- [ ] Web dashboard (React)

## Tech Stack

- **Go 1.24+** — standard library HTTP server (Go 1.22+ routing)
- **MySQL** — existing Khanza database
- **Zero dependencies** selain `go-sql-driver/mysql` dan `godotenv`

## License

Internal — RSAZ / ITRSHAA
