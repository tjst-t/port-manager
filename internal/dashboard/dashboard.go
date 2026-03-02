package dashboard

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"time"

	"github.com/tjst-t/port-manager/internal/config"
	"github.com/tjst-t/port-manager/internal/db"
)

// StatusChecker is a function that checks if a lease's process is alive.
// Used by the live dashboard to show real-time status.
type StatusChecker func(lease db.Lease) bool

// LeaseView represents a lease for dashboard rendering.
type LeaseView struct {
	Name     string
	Project  string
	Worktree string
	Port     int
	Hostname string
	Expose   bool
	State    string
	IsAlive  bool
}

// PermanentView represents a permanent service for dashboard rendering.
type PermanentView struct {
	Name   string
	Port   int
	Expose bool
}

// DashboardData contains all data needed to render the dashboard.
type DashboardData struct {
	Leases       []LeaseView
	Permanents   []PermanentView
	DomainSuffix string
	IsLive       bool
	GeneratedAt  string
}

// BuildDashboardData constructs the view model for the dashboard template.
// If checker is nil, IsAlive defaults to true for active leases.
func BuildDashboardData(leases []db.Lease, permanents []config.PermanentService, domainSuffix string, checker StatusChecker, isLive bool) DashboardData {
	// Filter out stale leases — dashboard is an access link collection
	var views []LeaseView
	for _, l := range leases {
		if l.State == "stale" {
			continue
		}
		alive := true
		if checker != nil {
			alive = checker(l)
		}
		views = append(views, LeaseView{
			Name:     l.Name,
			Project:  l.Project,
			Worktree: l.Worktree,
			Port:     l.Port,
			Hostname: l.Hostname,
			Expose:   l.Expose,
			State:    l.State,
			IsAlive:  alive,
		})
	}

	var permViews []PermanentView
	for _, p := range permanents {
		permViews = append(permViews, PermanentView{
			Name:   p.Name,
			Port:   p.Port,
			Expose: p.Expose,
		})
	}

	return DashboardData{
		Leases:       views,
		Permanents:   permViews,
		DomainSuffix: domainSuffix,
		IsLive:       isLive,
		GeneratedAt:  time.Now().Format("2006-01-02 15:04:05"),
	}
}

// RenderHTML renders the dashboard HTML from the given data.
func RenderHTML(data DashboardData) (string, error) {
	tmpl, err := template.New("dashboard").Funcs(funcMap()).Parse(dashboardTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}

// Generate creates a static HTML dashboard at outputDir/index.html.
func Generate(outputDir string, leases []db.Lease, permanents []config.PermanentService, domainSuffix string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	data := BuildDashboardData(leases, permanents, domainSuffix, nil, false)
	htmlContent, err := RenderHTML(data)
	if err != nil {
		return fmt.Errorf("rendering dashboard: %w", err)
	}

	path := filepath.Join(outputDir, "index.html")
	if err := os.WriteFile(path, []byte(htmlContent), 0644); err != nil {
		return fmt.Errorf("writing dashboard: %w", err)
	}

	return nil
}

func funcMap() template.FuncMap {
	return template.FuncMap{
		"exposeBadge": func(expose bool) template.HTML {
			if expose {
				return `<span class="badge badge-yes">yes</span>`
			}
			return `<span class="badge badge-no">no</span>`
		},
		"statusHTML": func(v LeaseView) template.HTML {
			if v.IsAlive {
				return `<span class="active">●</span> active`
			}
			return `<span class="stale">○</span> not running`
		},
		"leaseLink": func(v LeaseView, domainSuffix string) template.HTML {
			if v.Expose {
				fqdn := template.HTMLEscapeString(v.Hostname) + "." + template.HTMLEscapeString(domainSuffix)
				return template.HTML(fmt.Sprintf(`<a href="https://%s" target="_blank">%s</a>`, fqdn, fqdn))
			}
			return template.HTML(fmt.Sprintf("%d", v.Port))
		},
		"permanentLink": func(v PermanentView, domainSuffix string) template.HTML {
			if v.Expose {
				fqdn := template.HTMLEscapeString(v.Name) + "." + template.HTMLEscapeString(domainSuffix)
				return template.HTML(fmt.Sprintf(`<a href="https://%s" target="_blank">%s</a>`, fqdn, fqdn))
			}
			return template.HTML(fmt.Sprintf("%d", v.Port))
		},
	}
}

const dashboardTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
{{- if .IsLive}}
<meta http-equiv="refresh" content="30">
{{- end}}
<title>portman dashboard</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
body{font-family:system-ui,-apple-system,sans-serif;background:#f5f5f5;color:#333;padding:20px}
h1{margin-bottom:20px;font-size:1.5rem}
h2{margin:20px 0 10px;font-size:1.1rem;color:#555}
table{width:100%;border-collapse:collapse;background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 1px 3px rgba(0,0,0,.1);margin-bottom:20px}
th,td{padding:8px 12px;text-align:left;border-bottom:1px solid #eee}
th{background:#fafafa;font-weight:600;font-size:.85rem;color:#666}
td{font-size:.9rem}
a{color:#0066cc;text-decoration:none}
a:hover{text-decoration:underline}
.active{color:#22c55e}
.stale{color:#f59e0b}
.permanent{color:#8b5cf6}
.badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:.75rem;font-weight:600}
.badge-yes{background:#dcfce7;color:#166534}
.badge-no{background:#f3f4f6;color:#6b7280}
footer{margin-top:20px;font-size:.75rem;color:#999}
</style>
</head>
<body>
<h1>portman dashboard</h1>
<h2>Leases</h2>
<table>
<tr><th>Name</th><th>Project</th><th>Branch</th><th>Port</th><th>Status</th><th>Expose</th><th>Link</th></tr>
{{- if not .Leases}}
<tr><td colspan="7" style="text-align:center;color:#999">No active leases</td></tr>
{{- end}}
{{- range .Leases}}
<tr><td>{{.Name}}</td><td>{{.Project}}</td><td>{{.Worktree}}</td><td>{{.Port}}</td><td>{{statusHTML .}}</td><td>{{exposeBadge .Expose}}</td><td>{{leaseLink . $.DomainSuffix}}</td></tr>
{{- end}}
</table>
{{- if .Permanents}}
<h2>Permanent Services</h2>
<table>
<tr><th>Name</th><th>Port</th><th>Expose</th><th>Link</th></tr>
{{- range .Permanents}}
<tr><td><span class="permanent">&#9733;</span> {{.Name}}</td><td>{{.Port}}</td><td>{{exposeBadge .Expose}}</td><td>{{permanentLink . $.DomainSuffix}}</td></tr>
{{- end}}
</table>
{{- end}}
<footer>Generated by portman at {{.GeneratedAt}}</footer>
</body>
</html>
`
