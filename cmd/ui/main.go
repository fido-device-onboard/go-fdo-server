// SPDX-FileCopyrightText: (C) 2025
// SPDX-License-Identifier: Apache 2.0

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"
)

// Endpoints are configured at runtime via UI inputs (no defaults)
var (
	manufacturingBase = ""
	ownerBase         = ""
	listenAddr        = envOr("UI_ADDR", "127.0.0.1:8088")
)

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	// Flags to pre-seed endpoints (optional); UI still provides overrides at request time
	fdoIP := flag.String("fdo-ip", "", "FDO services IP/host (sets MFG to http://IP:8038 and OWNER to http://IP:8043)")
	mfg := flag.String("mfg", manufacturingBase, "Manufacturing base URL (e.g., http://127.0.0.1:8038)")
	owner := flag.String("owner", ownerBase, "Owner base URL (e.g., http://127.0.0.1:8043)")
	ui := flag.String("listen", listenAddr, "UI listen address (host:port)")
	flag.Parse()

	if *fdoIP != "" {
		manufacturingBase = fmt.Sprintf("http://%s:8038", *fdoIP)
		ownerBase = fmt.Sprintf("http://%s:8043", *fdoIP)
	}
	if *mfg != "" {
		manufacturingBase = *mfg
	}
	if *owner != "" {
		ownerBase = *owner
	}
	if *ui != "" {
		listenAddr = *ui
	}

	mux := http.NewServeMux()

	// UI
	mux.HandleFunc("/", handleIndex)

	// API proxy helpers (same-origin for browser)
	mux.HandleFunc("/api/ov/list", handleList)
	mux.HandleFunc("/api/owner/vouchers", handleOwnerVouchers)
	mux.HandleFunc("/api/ov/", handleVoucherPEM) // /api/ov/{guid}.pem
	mux.HandleFunc("/api/to0/", handleTO0)       // /api/to0/{guid}
	mux.HandleFunc("/api/submit/", handleSubmit) // /api/submit/{guid}

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	log.Printf("UI listening on http://%s", listenAddr)
	log.Fatal(srv.ListenAndServe())
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

// handleList proxies Manufacturing /vouchers to return JSON list (no CBOR)
func handleList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	base := q.Get("mfg")
	if base == "" {
		http.Error(w, "missing mfg base", http.StatusBadRequest)
		return
	}
	url := base + "/api/v1/vouchers"
	params := make([]string, 0, 2)
	if g := q.Get("guid"); g != "" {
		params = append(params, "guid="+g)
	}
	if d := q.Get("device_info"); d != "" {
		params = append(params, "device_info="+d)
	}
	if len(params) > 0 {
		url += "?" + params[0]
		for i := 1; i < len(params); i++ {
			url += "&" + params[i]
		}
	}
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, fmt.Sprintf("mfg list error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleOwnerVouchers proxies Owner /owner/vouchers to return JSON list (no CBOR)
func handleOwnerVouchers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	base := q.Get("owner")
	if base == "" {
		http.Error(w, "missing owner base", http.StatusBadRequest)
		return
	}
	url := base + "/api/v1/owner/vouchers"
	params := make([]string, 0, 2)
	if g := q.Get("guid"); g != "" {
		params = append(params, "guid="+g)
	}
	if d := q.Get("device_info"); d != "" {
		params = append(params, "device_info="+d)
	}
	if len(params) > 0 {
		url += "?" + params[0]
		for i := 1; i < len(params); i++ {
			url += "&" + params[i]
		}
	}
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, fmt.Sprintf("owner list error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleVoucherPEM proxies Manufacturing /vouchers/{guid} to download PEM
func handleVoucherPEM(w http.ResponseWriter, r *http.Request) {
	// path: /api/ov/{guid}.pem
	guid := r.URL.Path[len("/api/ov/"):]
	if guid == "" {
		http.Error(w, "missing guid", http.StatusBadRequest)
		return
	}
	// Accept both {guid} and {guid}.pem
	if len(guid) > 4 && guid[len(guid)-4:] == ".pem" {
		guid = guid[:len(guid)-4]
	}
	q := r.URL.Query()
	base := q.Get("mfg")
	if base == "" {
		http.Error(w, "missing mfg base", http.StatusBadRequest)
		return
	}
	url := fmt.Sprintf("%s/api/v1/vouchers/%s", base, guid)
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, fmt.Sprintf("mfg pem error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	if ct := resp.Header.Get("Content-Type"); ct == "" {
		w.Header().Set("Content-Type", "application/x-pem-file")
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleTO0 triggers Owner TO0 for a GUID
func handleTO0(w http.ResponseWriter, r *http.Request) {
	// path: /api/to0/{guid}
	guid := r.URL.Path[len("/api/to0/"):]
	if guid == "" {
		http.Error(w, "missing guid", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	base := q.Get("owner")
	if base == "" {
		http.Error(w, "missing owner base", http.StatusBadRequest)
		return
	}
	url := fmt.Sprintf("%s/api/v1/to0/%s", base, guid)
	resp, err := http.Get(url)
	if err != nil {
		http.Error(w, fmt.Sprintf("owner to0 error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// handleSubmit fetches voucher PEM from Manufacturing and posts it to Owner
func handleSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	guid := r.URL.Path[len("/api/submit/"):]
	if guid == "" {
		http.Error(w, "missing guid", http.StatusBadRequest)
		return
	}
	q := r.URL.Query()
	mfgBase := q.Get("mfg")
	if mfgBase == "" {
		http.Error(w, "missing mfg base", http.StatusBadRequest)
		return
	}
	ownerBaseLocal := q.Get("owner")
	if ownerBaseLocal == "" {
		http.Error(w, "missing owner base", http.StatusBadRequest)
		return
	}
	// Get PEM from MFG
	mfgURL := fmt.Sprintf("%s/api/v1/vouchers/%s", mfgBase, guid)
	mfgResp, err := http.Get(mfgURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("fetch pem error: %v", err), http.StatusBadGateway)
		return
	}
	defer mfgResp.Body.Close()
	if mfgResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(mfgResp.Body)
		http.Error(w, fmt.Sprintf("mfg returned %d: %s", mfgResp.StatusCode, string(body)), http.StatusBadGateway)
		return
	}
	pemBytes, err := io.ReadAll(mfgResp.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read pem error: %v", err), http.StatusBadGateway)
		return
	}
	// POST to Owner
	ownerURL := fmt.Sprintf("%s/api/v1/owner/vouchers", ownerBaseLocal)
	req, err := http.NewRequest(http.MethodPost, ownerURL, bytes.NewReader(pemBytes))
	if err != nil {
		http.Error(w, fmt.Sprintf("submit build req error: %v", err), http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/x-pem-file")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("owner submit error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>FDO Vouchers UI</title>
  <style>
    body { font-family: system-ui, sans-serif; margin: 24px; }
    h1 { font-size: 20px; }
    h2 { font-size: 16px; margin-top: 18px; }
    table { border-collapse: collapse; width: 100%; margin-top: 8px; }
    th, td { border: 1px solid #ddd; padding: 8px; font-size: 14px; }
    th { background: #f3f3f3; text-align: left; }
    input, button { font-size: 14px; padding: 6px 10px; }
    .row-actions { display: flex; gap: 8px; align-items: center; }
    .toolbar { display:flex; gap:8px; align-items:center; flex-wrap: wrap; }
    .status { margin-left: 8px; color: #555; }
    .endpoints { display:flex; gap:8px; align-items:center; flex-wrap: wrap; margin-top:8px; }
    .endpoints input { width: 340px; }
  </style>
  <script>
    function updateActions() {
      const mfg = document.getElementById('mfgBase').value.trim();
      const owner = document.getElementById('ownerBase').value.trim();
      const canRun = !!mfg || !!owner;
      document.getElementById('btnRefresh').disabled = !canRun;
      document.getElementById('status').textContent = canRun ? '' : 'Set at least one endpoint to enable refresh';
    }

    async function fetchList() {
      const deviceInfo = document.getElementById('filterDeviceInfo').value.trim();
      const guid = document.getElementById('filterGuid').value.trim();
      const mfg = document.getElementById('mfgBase').value.trim();
      const owner = document.getElementById('ownerBase').value.trim();
      if (!mfg && !owner) { updateActions(); return; }

      let mfgList = [];
      let ownerList = [];

      const tasks = [];
      if (mfg) {
        const mfgURL = '/api/ov/list?' + new URLSearchParams({ mfg, ...(deviceInfo ? { device_info: deviceInfo } : {}), ...(guid ? { guid } : {}) });
        tasks.push(fetch(mfgURL).then(r => r.json()).then(d => { mfgList = d; }).catch(() => { mfgList = []; }));
      } else {
        document.getElementById('tbody-mfg').innerHTML = '';
      }

      if (owner) {
        const ownerURL = '/api/owner/vouchers?' + new URLSearchParams({ owner, ...(deviceInfo ? { device_info: deviceInfo } : {}), ...(guid ? { guid } : {}) });
        tasks.push(fetch(ownerURL).then(r => r.json()).then(d => { ownerList = d; }).catch(() => { ownerList = []; }));
      } else {
        document.getElementById('tbody-owner').innerHTML = '';
      }

      await Promise.all(tasks);

      if (mfg) renderMfgTable(mfgList);
      if (owner) renderOwnerTable(ownerList);

      const m = mfg ? (mfgList.length + ' mfg') : 'mfg: n/a';
      const o = owner ? (ownerList.length + ' owner') : 'owner: n/a';
      document.getElementById('status').textContent = 'Loaded ' + m + ' vouchers, ' + o + ' vouchers';
    }

    function formatTsEpoch(n) {
      if (n === null || n === undefined || n === '') return '';
      const v = typeof n === 'number' ? n : parseInt(n, 10);
      if (!Number.isFinite(v)) return '';
      const ms = v < 1e12 ? (v * 1000) : Math.floor(v / 1000);
      return new Date(ms).toLocaleString();
    }

    function renderMfgTable(list) {
      const tbody = document.getElementById('tbody-mfg');
      const mfg = document.getElementById('mfgBase').value.trim();
      const owner = document.getElementById('ownerBase').value.trim();
      tbody.innerHTML = '';
      for (const item of list) {
        const tr = document.createElement('tr');
        const guidCell = document.createElement('td'); guidCell.textContent = item.guid;
        const infoCell = document.createElement('td'); infoCell.textContent = item.device_info || '';
        const cAt = document.createElement('td'); cAt.textContent = formatTsEpoch(item.created_at);
        const uAt = document.createElement('td'); uAt.textContent = formatTsEpoch(item.updated_at);
        const actions = document.createElement('td'); actions.className = 'row-actions';

        const btnPem = document.createElement('button'); btnPem.textContent = 'Export PEM'; btnPem.disabled = !mfg;
        btnPem.onclick = () => { if (!mfg) return; window.location = '/api/ov/' + item.guid + '.pem?mfg=' + encodeURIComponent(mfg); };

        const btnSubmit = document.createElement('button'); btnSubmit.textContent = 'Submit to Owner'; btnSubmit.disabled = !owner; btnSubmit.style.marginRight = '8px'; btnSubmit.style.display = 'none';
        btnSubmit.onclick = async () => {
          if (!owner) return;
          const res = await fetch('/api/submit/' + item.guid + '?mfg=' + encodeURIComponent(mfg) + '&owner=' + encodeURIComponent(owner), { method: 'POST' });
          const txt = await res.text();
          alert('Submit status: ' + res.status + '\n' + txt);
          if (res.ok) { btnSubmit.style.display = 'none'; }
        };

        if (owner) {
          fetch('/api/owner/vouchers?owner=' + encodeURIComponent(owner) + '&guid=' + encodeURIComponent(item.guid))
            .then(r => r.json())
            .then(arr => {
              if (Array.isArray(arr) && arr.length === 0) {
                btnSubmit.style.display = '';
              } else {
                btnSubmit.style.display = 'none';
              }
            })
            .catch(() => { btnSubmit.style.display = 'none'; });
        }

        actions.appendChild(btnPem);
        actions.appendChild(btnSubmit);
        tr.appendChild(guidCell);
        tr.appendChild(infoCell);
        tr.appendChild(cAt);
        tr.appendChild(uAt);
        tr.appendChild(actions);
        tbody.appendChild(tr);
      }
    }

    function renderOwnerTable(list) {
      const tbody = document.getElementById('tbody-owner');
      const owner = document.getElementById('ownerBase').value.trim();
      tbody.innerHTML = '';
      for (const item of list) {
        const tr = document.createElement('tr');
        const guidCell = document.createElement('td'); guidCell.textContent = item.guid;
        const infoCell = document.createElement('td'); infoCell.textContent = item.device_info || '';
        const cAt = document.createElement('td'); cAt.textContent = formatTsEpoch(item.created_at);
        const uAt = document.createElement('td'); uAt.textContent = formatTsEpoch(item.updated_at);
        const actions = document.createElement('td'); actions.className = 'row-actions';

        const btnTo0 = document.createElement('button'); btnTo0.textContent = 'Run TO0'; btnTo0.disabled = !owner;
        btnTo0.onclick = async () => {
          if (!owner) return;
          const res = await fetch('/api/to0/' + item.guid + '?owner=' + encodeURIComponent(owner));
          const txt = await res.text();
          alert('TO0 status: ' + res.status + '\n' + txt);
        };

        actions.appendChild(btnTo0);
        tr.appendChild(guidCell);
        tr.appendChild(infoCell);
        tr.appendChild(cAt);
        tr.appendChild(uAt);
        tr.appendChild(actions);
        tbody.appendChild(tr);
      }
    }

    window.addEventListener('DOMContentLoaded', () => {
      document.getElementById('mfgBase').addEventListener('input', updateActions);
      document.getElementById('ownerBase').addEventListener('input', updateActions);
      updateActions();
    });
  </script>
</head>
<body>
  <h1>FDO Vouchers UI</h1>
  <div class="toolbar">
    <label>device_info <input id="filterDeviceInfo" placeholder="e.g. gotest1"/></label>
    <label>guid <input id="filterGuid" placeholder="32-hex GUID"/></label>
    <button id="btnRefresh" onclick="fetchList()">Refresh</button>
    <span id="status" class="status"></span>
  </div>
  <div class="endpoints">
    <label>Manufacturing base <input id="mfgBase" placeholder="http://host:8038"/></label>
    <label>Owner base <input id="ownerBase" placeholder="http://host:8043"/></label>
  </div>

  <h2>Manufacturing Vouchers</h2>
  <table>
    <thead>
      <tr>
        <th>GUID</th>
        <th>Device Info</th>
        <th>Created</th>
        <th>Updated</th>
        <th>Actions</th>
      </tr>
    </thead>
    <tbody id="tbody-mfg"></tbody>
  </table>

  <h2>Owner Vouchers</h2>
  <table>
    <thead>
      <tr>
        <th>GUID</th>
        <th>Device Info</th>
        <th>Created</th>
        <th>Updated</th>
        <th>Actions</th>
      </tr>
    </thead>
    <tbody id="tbody-owner"></tbody>
  </table>
</body>
</html>
`
