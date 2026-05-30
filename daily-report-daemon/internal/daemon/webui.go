package daemon

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// WebUI serves a simple local dashboard for reports and configuration.
type WebUI struct {
	DataDir string
	Port    int
}

// NewWebUI creates a web UI server.
func NewWebUI(dataDir string, port int) *WebUI {
	return &WebUI{DataDir: dataDir, Port: port}
}

// Start launches the web UI server (blocking).
func (ui *WebUI) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/", ui.handleIndex)
	mux.HandleFunc("/reports/", ui.handleReports)
	mux.HandleFunc("/context/", ui.handleContext)
	mux.HandleFunc("/api/status", ui.handleAPIStatus)

	addr := fmt.Sprintf(":%d", ui.Port)
	fmt.Printf("[webui] Dashboard at http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func (ui *WebUI) handleIndex(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>daily-report-daemon</title>
<style>
body { font-family: -apple-system, sans-serif; max-width: 800px; margin: 0 auto; padding: 20px; background: #f5f5f5; }
.card { background: white; border-radius: 8px; padding: 20px; margin: 10px 0; box-shadow: 0 1px 3px rgba(0,0,0,0.1); }
h1 { color: #333; }
a { color: #0066cc; text-decoration: none; }
a:hover { text-decoration: underline; }
.status { display: inline-block; padding: 4px 8px; border-radius: 4px; font-size: 12px; }
.running { background: #e6ffe6; color: #006600; }
.stopped { background: #ffe6e6; color: #660000; }
pre { background: #f0f0f0; padding: 10px; border-radius: 4px; overflow-x: auto; font-size: 12px; }
</style>
</head>
<body>
<h1>📊 daily-report-daemon</h1>
<div class="card">
<h2>今日报告</h2>
<p><a href="/reports/">查看所有报告</a></p>
</div>
<div class="card">
<h2>Agent Context</h2>
<p><a href="/context/">查看 AGENTS.generated.md</a></p>
</div>
<div class="card">
<h2>系统状态</h2>
<p><a href="/api/status">API 状态</a></p>
</div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(html))
}

func (ui *WebUI) handleReports(w http.ResponseWriter, r *http.Request) {
	reportsDir := filepath.Join(ui.DataDir, "reports")
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		http.Error(w, "Reports not found", 404)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, "<h1>报告列表</h1><ul>")
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".md") {
			path := filepath.Join(reportsDir, e.Name())
			data, _ := os.ReadFile(path)
			fmt.Fprintf(w, "<li><a href='/reports/%s'>%s</a></li>", e.Name(), e.Name())
			_ = data
		}
	}
	fmt.Fprintf(w, "</ul>")
}

func (ui *WebUI) handleContext(w http.ResponseWriter, r *http.Request) {
	ctxPath := filepath.Join(ui.DataDir, "context", "AGENTS.generated.md")
	data, err := os.ReadFile(ctxPath)
	if err != nil {
		http.Error(w, "AGENTS.generated.md not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(data)
}

func (ui *WebUI) handleAPIStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	status := fmt.Sprintf(`{"status":"ok","data_dir":"%s","reports":[],"context":[]}`, ui.DataDir)
	w.Write([]byte(status))
}
