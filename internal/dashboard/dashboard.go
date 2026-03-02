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

// BranchGroup groups leases under a single branch (worktree).
type BranchGroup struct {
	Name   string
	Leases []LeaseView
}

// ProjectGroup groups branches under a single project.
type ProjectGroup struct {
	Name     string
	Branches []BranchGroup
}

// DashboardData contains all data needed to render the dashboard.
type DashboardData struct {
	Projects     []ProjectGroup
	Leases       []LeaseView // flat list kept for backward compat (tests)
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
		Projects:     groupByProject(views),
		Leases:       views,
		Permanents:   permViews,
		DomainSuffix: domainSuffix,
		IsLive:       isLive,
		GeneratedAt:  time.Now().Format("2006-01-02 15:04:05"),
	}
}

// groupByProject groups flat lease views into Project → Branch hierarchy,
// preserving insertion order.
func groupByProject(views []LeaseView) []ProjectGroup {
	projectOrder := []string{}
	projectMap := map[string]*ProjectGroup{}

	for _, v := range views {
		pg, ok := projectMap[v.Project]
		if !ok {
			pg = &ProjectGroup{Name: v.Project}
			projectMap[v.Project] = pg
			projectOrder = append(projectOrder, v.Project)
		}

		// Find or create branch within project
		found := false
		for i := range pg.Branches {
			if pg.Branches[i].Name == v.Worktree {
				pg.Branches[i].Leases = append(pg.Branches[i].Leases, v)
				found = true
				break
			}
		}
		if !found {
			pg.Branches = append(pg.Branches, BranchGroup{
				Name:   v.Worktree,
				Leases: []LeaseView{v},
			})
		}
	}

	result := make([]ProjectGroup, 0, len(projectOrder))
	for _, name := range projectOrder {
		result = append(result, *projectMap[name])
	}
	return result
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
		"statusDot": func(v LeaseView) template.HTML {
			if v.IsAlive {
				return `<span class="dot dot-active" title="active"></span>`
			}
			return `<span class="dot dot-inactive" title="not running"></span>`
		},
		"nameLink": func(v LeaseView, domainSuffix string) template.HTML {
			escaped := template.HTMLEscapeString(v.Name)
			if v.Expose {
				fqdn := template.HTMLEscapeString(v.Hostname) + "." + template.HTMLEscapeString(domainSuffix)
				return template.HTML(fmt.Sprintf(`<a href="https://%s" target="_blank">%s</a>`, fqdn, escaped))
			}
			return template.HTML(escaped)
		},
		"branchLink": func(bg BranchGroup, domainSuffix string) template.HTML {
			escaped := template.HTMLEscapeString(bg.Name)
			// If exactly one lease and it's exposed, the branch name is clickable
			if len(bg.Leases) == 1 && bg.Leases[0].Expose {
				fqdn := template.HTMLEscapeString(bg.Leases[0].Hostname) + "." + template.HTMLEscapeString(domainSuffix)
				return template.HTML(fmt.Sprintf(`<a href="https://%s" target="_blank">%s</a>`, fqdn, escaped))
			}
			return template.HTML(escaped)
		},
		"permanentNameLink": func(v PermanentView, domainSuffix string) template.HTML {
			escaped := template.HTMLEscapeString(v.Name)
			if v.Expose {
				fqdn := template.HTMLEscapeString(v.Name) + "." + template.HTMLEscapeString(domainSuffix)
				return template.HTML(fmt.Sprintf(`<a href="https://%s" target="_blank">%s</a>`, fqdn, escaped))
			}
			return template.HTML(escaped)
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
:root {
  --bg: #f0f2f5;
  --card-bg: #fff;
  --border: #e2e5e9;
  --text: #1a1d21;
  --text-secondary: #5f6368;
  --text-muted: #9aa0a6;
  --accent: #1a73e8;
  --green: #34a853;
  --red: #ea4335;
  --badge-yes-bg: #e6f4ea;
  --badge-yes-fg: #137333;
  --badge-no-bg: #f1f3f4;
  --badge-no-fg: #80868b;
  --shadow: 0 1px 3px rgba(0,0,0,.08), 0 1px 2px rgba(0,0,0,.04);
  --radius: 10px;
}
*, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  background: var(--bg);
  color: var(--text);
  padding: 24px;
  max-width: 960px;
  margin: 0 auto;
  line-height: 1.5;
}
h1 {
  font-size: 1.4rem;
  font-weight: 700;
  margin-bottom: 24px;
  letter-spacing: -.01em;
}
.section-title {
  font-size: .8rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: .06em;
  color: var(--text-muted);
  margin-bottom: 12px;
}

/* Project card */
.project-card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  margin-bottom: 16px;
  overflow: hidden;
}
.project-header {
  padding: 12px 16px;
  font-weight: 600;
  font-size: .95rem;
  background: #fafbfc;
  border-bottom: 1px solid var(--border);
}

/* Branch row */
.branch-row {
  display: flex;
  border-bottom: 1px solid var(--border);
}
.branch-row:last-child { border-bottom: none; }
.branch-name {
  width: 160px;
  min-width: 120px;
  padding: 0 16px;
  display: flex;
  align-items: center;
  font-size: .85rem;
  font-weight: 500;
  color: var(--text-secondary);
  background: #fafbfc;
  border-right: 1px solid var(--border);
  word-break: break-all;
}
.branch-name a { color: var(--accent); text-decoration: none; }
.branch-name a:hover { text-decoration: underline; }
.branch-services {
  flex: 1;
  min-width: 0;
}

/* Service table inside branch */
.branch-services table {
  width: 100%;
  border-collapse: collapse;
}
.branch-services th,
.branch-services td {
  padding: 8px 12px;
  text-align: left;
  font-size: .85rem;
  border-bottom: 1px solid var(--border);
}
.branch-services tr:last-child th,
.branch-services tr:last-child td { border-bottom: none; }
.branch-services th {
  font-weight: 600;
  font-size: .75rem;
  text-transform: uppercase;
  letter-spacing: .04em;
  color: var(--text-muted);
  background: #fafbfc;
}

/* Name column links */
.branch-services td a { color: var(--accent); text-decoration: none; font-weight: 500; }
.branch-services td a:hover { text-decoration: underline; }

/* Status dot */
.dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
}
.dot-active { background: var(--green); }
.dot-inactive { background: var(--red); }

/* Expose badge */
.badge {
  display: inline-block;
  padding: 2px 8px;
  border-radius: 4px;
  font-size: .7rem;
  font-weight: 600;
  letter-spacing: .02em;
}
.badge-yes { background: var(--badge-yes-bg); color: var(--badge-yes-fg); }
.badge-no { background: var(--badge-no-bg); color: var(--badge-no-fg); }

/* Empty state */
.empty-state {
  padding: 32px 16px;
  text-align: center;
  color: var(--text-muted);
  font-size: .9rem;
}

/* Permanent table */
.permanent-card {
  background: var(--card-bg);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  box-shadow: var(--shadow);
  overflow: hidden;
}
.permanent-card table {
  width: 100%;
  border-collapse: collapse;
}
.permanent-card th,
.permanent-card td {
  padding: 10px 16px;
  text-align: left;
  font-size: .85rem;
  border-bottom: 1px solid var(--border);
}
.permanent-card tr:last-child td { border-bottom: none; }
.permanent-card th {
  font-weight: 600;
  font-size: .75rem;
  text-transform: uppercase;
  letter-spacing: .04em;
  color: var(--text-muted);
  background: #fafbfc;
}
.permanent-card td a { color: var(--accent); text-decoration: none; font-weight: 500; }
.permanent-card td a:hover { text-decoration: underline; }

footer {
  margin-top: 24px;
  font-size: .75rem;
  color: var(--text-muted);
}

/* Responsive: stack branch layout on narrow screens */
@media (max-width: 640px) {
  body { padding: 12px; }
  .branch-row { flex-direction: column; }
  .branch-name {
    width: 100%;
    min-width: unset;
    padding: 8px 12px;
    border-right: none;
    border-bottom: 1px solid var(--border);
  }
  .branch-services th,
  .branch-services td { padding: 6px 8px; font-size: .8rem; }
  .permanent-card th,
  .permanent-card td { padding: 8px 12px; font-size: .8rem; }
}
</style>
</head>
<body>
<h1>portman dashboard</h1>

<div class="section-title">Leases</div>
{{- if not .Projects}}
<div class="project-card">
  <div class="empty-state">No active leases</div>
</div>
{{- end}}
{{- range .Projects}}
<div class="project-card">
  <div class="project-header">{{.Name}}</div>
  {{- range .Branches}}
  <div class="branch-row">
    <div class="branch-name">{{branchLink . $.DomainSuffix}}</div>
    <div class="branch-services">
      <table>
        <tr><th>Name</th><th>Port</th><th>Status</th><th>Expose</th></tr>
        {{- range .Leases}}
        <tr>
          <td>{{nameLink . $.DomainSuffix}}</td>
          <td>{{.Port}}</td>
          <td>{{statusDot .}}</td>
          <td>{{exposeBadge .Expose}}</td>
        </tr>
        {{- end}}
      </table>
    </div>
  </div>
  {{- end}}
</div>
{{- end}}

{{- if .Permanents}}
<div class="section-title" style="margin-top:24px">Permanent</div>
<div class="permanent-card">
  <table>
    <tr><th>Name</th><th>Port</th><th>Expose</th></tr>
    {{- range .Permanents}}
    <tr>
      <td>{{permanentNameLink . $.DomainSuffix}}</td>
      <td>{{.Port}}</td>
      <td>{{exposeBadge .Expose}}</td>
    </tr>
    {{- end}}
  </table>
</div>
{{- end}}

<footer>Generated by portman at {{.GeneratedAt}}</footer>
</body>
</html>
`
