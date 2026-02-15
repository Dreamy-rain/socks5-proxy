package main

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"time"
)

type StatusServer struct {
	pool *ProxyPool
}

type StatusData struct {
	Total        int           `json:"total"`
	ActiveProxy  string        `json:"active_proxy"`
	ActiveRegion string        `json:"active_region"`
	LastScrape   string        `json:"last_scrape"`
	NextScrape   string        `json:"next_scrape"`
	Proxies      []ProxyStatus `json:"proxies"`
}

type ProxyStatus struct {
	Addr    string `json:"addr"`
	Country string `json:"country"`
	City    string `json:"city"`
	Active  bool   `json:"active"`
}

func NewStatusServer(pool *ProxyPool) *StatusServer {
	return &StatusServer{
		pool: pool,
	}
}

func (s *StatusServer) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/status", s.handleAPI)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/switch", s.handleSwitch)
	return http.ListenAndServe(addr, mux)
}

func (s *StatusServer) getStatusData() StatusData {
	proxies := s.pool.All()
	activeIdx := s.pool.CurrentIndex()
	last, next := getScrapeTimes()

	// Beijing timezone (UTC+8)
	beijingLoc := time.FixedZone("CST", 8*3600)

	var lastStr, nextStr string
	if !last.IsZero() {
		lastStr = last.In(beijingLoc).Format("2006-01-02 15:04:05")
	}
	if !next.IsZero() {
		nextStr = next.In(beijingLoc).Format("2006-01-02 15:04:05")
	}

	var ps []ProxyStatus
	for i, p := range proxies {
		ps = append(ps, ProxyStatus{
			Addr:    p.Addr(),
			Country: p.Country,
			City:    p.City,
			Active:  i == activeIdx,
		})
	}

	// Get active proxy info
	var activeProxy, activeRegion string
	if p, ok := s.pool.Current(); ok {
		activeProxy = p.Addr()
		activeRegion = p.Country
		if p.City != "" {
			activeRegion += ", " + p.City
		}
	} else {
		activeProxy = "None"
		activeRegion = "-"
	}

	return StatusData{
		Total:        len(proxies),
		ActiveProxy:  activeProxy,
		ActiveRegion: activeRegion,
		LastScrape:   lastStr,
		NextScrape:   nextStr,
		Proxies:      ps,
	}
}

func (s *StatusServer) handleAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.getStatusData())
}

func (s *StatusServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	TriggerRefresh()
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"refresh triggered"}`))
}

func (s *StatusServer) handleSwitch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	indexStr := r.URL.Query().Get("index")
	if indexStr != "" {
		index, err := strconv.Atoi(indexStr)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status":"invalid index"}`))
			return
		}
		if _, ok := s.pool.SwitchTo(index); ok {
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"status":"index out of range"}`))
		}
	} else {
		if _, ok := s.pool.SwitchNext(); ok {
			w.Write([]byte(`{"status":"ok"}`))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte(`{"status":"no proxies available"}`))
		}
	}
}

func (s *StatusServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data := s.getStatusData()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	dashboardTmpl.Execute(w, data)
}

var dashboardTmpl = template.Must(template.New("dashboard").Parse(`<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>SOCKS5 Pool Status</title>
<meta name="viewport" content="width=device-width,initial-scale=1">
<meta http-equiv="refresh" content="30">
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;padding:12px}
.container{max-width:800px;margin:0 auto}
h1{font-size:1.3rem;color:#38bdf8}
.current{background:#1e293b;border-radius:8px;padding:12px 16px;margin:12px 0;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px}
.current-info{font-size:0.9rem}
.current-info .addr{color:#4ade80;font-family:monospace;font-weight:bold}
.current-info .region{color:#94a3b8;font-size:0.8rem}
.badge{background:#065f46;color:#4ade80;padding:2px 8px;border-radius:4px;font-size:0.75rem;font-weight:bold}
.time-info{background:#1e293b;border-radius:8px;padding:12px 16px;margin:8px 0;display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px}
.time-item{font-size:0.8rem;color:#94a3b8}
.time-item span{color:#e2e8f0;font-family:monospace}
.btn{background:#38bdf8;color:#0f172a;border:none;padding:6px 14px;border-radius:6px;cursor:pointer;font-weight:bold;font-size:0.8rem}
.btn:hover{background:#7dd3fc}
.btn:disabled{background:#334155;color:#64748b;cursor:not-allowed}
.list{margin-top:12px}
.proxy-card{background:#1e293b;border-radius:8px;padding:12px 16px;margin:6px 0;cursor:pointer;display:flex;justify-content:space-between;align-items:center;transition:background 0.15s;border:2px solid transparent}
.proxy-card:hover{background:#334155}
.proxy-card.active{border-color:#4ade80;background:#1a2e1a}
.proxy-card .left{display:flex;align-items:center;gap:10px;min-width:0}
.proxy-card .idx{color:#64748b;font-size:0.8rem;width:20px;text-align:center;flex-shrink:0}
.proxy-card .addr{font-family:monospace;font-size:0.85rem;word-break:break-all}
.proxy-card .loc{color:#94a3b8;font-size:0.8rem}
.proxy-card .status{flex-shrink:0;font-size:0.75rem;font-weight:bold}
.proxy-card .status.in-use{color:#4ade80}
.proxy-card .status.standby{color:#64748b}
.note{color:#64748b;font-size:0.75rem;margin-top:10px;text-align:center}
.empty{text-align:center;padding:40px;color:#64748b}
.total{color:#94a3b8;font-size:0.85rem}
.gh-link{color:#64748b;text-decoration:none;display:inline-flex;align-items:center;gap:4px;font-size:0.8rem;transition:color 0.15s}
.gh-link:hover{color:#e2e8f0}
.gh-link svg{width:18px;height:18px;fill:currentColor}
</style>
</head>
<body>
<div class="container">
<div style="display:flex;justify-content:space-between;align-items:center;flex-wrap:wrap;gap:8px">
  <h1>SOCKS5 Proxy Pool</h1>
  <div style="display:flex;align-items:center;gap:12px">
    <a class="gh-link" href="https://github.com/Dreamy-rain/socks5-proxy" target="_blank" rel="noopener"><svg viewBox="0 0 16 16"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"/></svg></a>
    <span class="total">{{.Total}} proxies</span>
  </div>
</div>
<div class="current">
  <div class="current-info">
    <span class="badge">IN USE</span>
    <span class="addr">{{.ActiveProxy}}</span>
    <span class="region">{{.ActiveRegion}}</span>
  </div>
</div>
<div class="time-info">
  <div>
    <div class="time-item">Last: <span>{{if .LastScrape}}{{.LastScrape}}{{else}}N/A{{end}}</span></div>
    <div class="time-item">Next: <span>{{if .NextScrape}}{{.NextScrape}}{{else}}N/A{{end}}</span></div>
  </div>
  <button class="btn" onclick="doRefresh(this)">Refresh Pool</button>
</div>
{{if .Proxies}}
<div class="list">
{{range $i, $p := .Proxies}}
<div class="proxy-card{{if $p.Active}} active{{end}}" onclick="doSwitch({{$i}},this)">
  <div class="left">
    <span class="idx">{{$i}}</span>
    <div>
      <div class="addr">{{$p.Addr}}</div>
      <div class="loc">{{$p.Country}}{{if $p.City}}, {{$p.City}}{{end}}</div>
    </div>
  </div>
  <span class="status {{if $p.Active}}in-use{{else}}standby{{end}}">{{if $p.Active}}IN USE{{else}}standby{{end}}</span>
</div>
{{end}}
</div>
{{else}}
<p class="empty">No proxies available. Waiting for next scrape cycle...</p>
{{end}}
<p class="note">Auto-refresh 30s | Beijing Time (UTC+8) | Click proxy to switch | Google-verified</p>
<p class="note">Proxy source: <a href="https://socks5-proxy.github.io/" target="_blank" rel="noopener" style="color:#38bdf8;text-decoration:none">socks5-proxy.github.io</a></p>
</div>
<script>
function doSwitch(idx, el) {
  if (el.classList.contains('active')) return;
  el.style.opacity='0.5';
  fetch('/api/switch?index='+idx).then(function(res) {
    if (res.ok) { location.reload(); }
    else { el.style.opacity='1'; alert('Switch failed'); }
  }).catch(function() { el.style.opacity='1'; });
}
function doRefresh(btn) {
  btn.disabled = true;
  btn.textContent = 'Refreshing...';
  fetch('/api/refresh').then(function() {
    setTimeout(function() { location.reload(); }, 15000);
  }).catch(function() {
    btn.disabled = false;
    btn.textContent = 'Refresh Pool';
  });
}
</script>
</body>
</html>`))
