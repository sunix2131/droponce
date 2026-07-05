package receiverweb

import (
	"encoding/json"
	"html/template"
	"io"
	"strconv"
)

type PageData struct {
	Title        string
	Mode         string
	FileName     string
	SizeBytes    int64
	Expires      string
	Remaining    int
	DownloadPath string
	TrustNote    string
}

func Render(w io.Writer, data PageData) error {
	data.Title = fallback(data.Title, "DropOnce")
	data.Mode = fallback(data.Mode, "Временная передача файла")
	data.TrustNote = fallback(data.TrustNote, "Ссылка временная. Не передавайте её незнакомым людям.")
	return page.Execute(w, data)
}

func ManifestJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"name":             "DropOnce Receiver",
		"short_name":       "DropOnce",
		"description":      "Receive a temporary DropOnce file transfer.",
		"display":          "standalone",
		"orientation":      "portrait",
		"background_color": "#f5f4ef",
		"theme_color":      "#1f6b5c",
		"start_url":        ".",
		"scope":            "/",
		"icons": []map[string]string{
			{"src": "/drop-icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any maskable"},
		},
	})
}

func IconSVG() string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 128 128"><rect width="128" height="128" rx="24" fill="#1f6b5c"/><path d="M32 33h24v24H32V33Zm40 0h24v24H72V33ZM32 72h24v24H32V72Zm40 0h24v24H72V72Z" fill="#fff"/><path d="M58 58h12v12H58V58Zm-26 0h12v12H32V58Zm52 0h12v12H84V58ZM58 84h12v12H58V84Z" fill="#b7f4df"/></svg>`
}

func fallback(value, fallbackValue string) string {
	if value == "" {
		return fallbackValue
	}
	return value
}

func humanBytes(value int64) string {
	if value < 1024 {
		return strconv.FormatInt(value, 10) + " B"
	}
	units := []string{"KB", "MB", "GB", "TB"}
	n := float64(value)
	for _, unit := range units {
		n /= 1024
		if n < 1024 {
			return strconv.FormatFloat(n, 'f', 1, 64) + " " + unit
		}
	}
	return strconv.FormatFloat(n, 'f', 1, 64) + " PB"
}

var page = template.Must(template.New("receiver").Funcs(template.FuncMap{"humanBytes": humanBytes}).Parse(`<!doctype html>
<html lang="ru"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
<meta name="theme-color" content="#1f6b5c">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="apple-mobile-web-app-title" content="DropOnce">
<link rel="manifest" href="/app.webmanifest">
<link rel="icon" href="/drop-icon.svg" type="image/svg+xml">
<title>{{.Title}}</title>
<style>
:root{color-scheme:light dark;--bg:#f5f4ef;--panel:#fffefa;--text:#1f2933;--muted:#667085;--line:#d8d8ce;--accent:#1f6b5c;--accent2:#e4f4ee;--warn:#7a4b10}
@media (prefers-color-scheme:dark){:root{--bg:#151719;--panel:#202327;--text:#edf0f2;--muted:#b7c0c7;--line:#373d44;--accent:#6bd2bb;--accent2:#153a34;--warn:#f0c16b}}
*{box-sizing:border-box}body{margin:0;min-height:100dvh;background:var(--bg);color:var(--text);font-family:ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
main{width:min(100%,560px);margin:0 auto;padding:calc(22px + env(safe-area-inset-top)) 18px calc(24px + env(safe-area-inset-bottom))}
.top{display:flex;align-items:center;gap:12px;margin-bottom:22px}.logo{display:grid;place-items:center;width:42px;height:42px;border-radius:12px;background:var(--accent);color:#fff;font-weight:900}.brand{display:grid}.brand strong{font-size:22px}.brand span{color:var(--muted);font-size:14px}
.panel{background:var(--panel);border:1px solid var(--line);border-radius:14px;padding:20px;box-shadow:0 20px 50px rgba(20,30,25,.08)}.file{display:grid;gap:10px}.fileName{font-size:24px;line-height:1.16;font-weight:850;overflow-wrap:anywhere}.meta{display:grid;gap:8px;margin:10px 0 18px}.row{display:flex;justify-content:space-between;gap:16px;color:var(--muted);font-size:15px}.row strong{color:var(--text);font-weight:750;text-align:right}
.download{display:flex;align-items:center;justify-content:center;min-height:54px;border-radius:10px;background:var(--accent);color:#fff;text-decoration:none;font-weight:850;font-size:17px;box-shadow:0 16px 32px rgba(31,107,92,.22)}.download:active{transform:translateY(1px)}
.note{margin-top:14px;padding:12px;border-radius:10px;background:var(--accent2);color:var(--text);line-height:1.45;font-size:14px}.warn{margin-top:12px;color:var(--muted);font-size:13px;line-height:1.45}.steps{display:grid;grid-template-columns:repeat(3,1fr);gap:8px;margin:18px 0}.step{padding:9px 8px;border-radius:999px;background:var(--panel);border:1px solid var(--line);color:var(--muted);text-align:center;font-size:12px;font-weight:800}.step.on{background:var(--accent2);color:var(--accent);border-color:transparent}
</style></head><body><main>
<div class="top"><div class="logo">D1</div><div class="brand"><strong>DropOnce</strong><span>{{.Mode}}</span></div></div>
<div class="steps"><div class="step on">QR открыт</div><div class="step on">Ссылка активна</div><div class="step">Скачать</div></div>
<section class="panel"><div class="file"><div class="fileName">{{.FileName}}</div>
<div class="meta"><div class="row"><span>Размер</span><strong>{{humanBytes .SizeBytes}}</strong></div><div class="row"><span>Действует до</span><strong>{{.Expires}}</strong></div><div class="row"><span>Осталось скачиваний</span><strong>{{.Remaining}}</strong></div></div>
<a class="download" href="{{.DownloadPath}}">Скачать файл</a></div><div class="note">{{.TrustNote}}</div></section>
<p class="warn">Оставьте эту вкладку открытой, пока браузер начинает скачивание. Прогресс покажет сам браузер.</p>
</main></body></html>`))
