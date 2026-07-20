package report

import (
	"bytes"
	"fmt"
	"html/template"
	"strings"
)

// The style guide's palette, as literals.
//
// An email cannot use CSS custom properties: Outlook and Gmail strip or ignore
// most of a <style> block, so every rule has to be inlined on the element. The
// values are --text, --text-muted, --border, --accent, --positive, --negative.
const (
	colorText     = "#0a0a0a"
	colorMuted    = "#71717a"
	colorSubtle   = "#a1a1aa"
	colorBorder   = "#e4e4e7"
	colorSurface  = "#fafafa"
	colorAccent   = "#2563eb"
	colorPositive = "#16a34a"
	colorNegative = "#dc2626"
	colorWarning  = "#d97706"

	// Geist will not load in an email client, so the stack degrades to whatever
	// the reader has. The report is light-only: dark-mode email support is a
	// minefield of client-specific inversion, and a light email on a dark
	// client is legible while a half-inverted one is not.
	fontStack = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif"
	monoStack = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
)

// Subject returns the email's subject line.
//
// It names the site and the month, because it lands in an inbox next to
// everything else and has to be identifiable without being opened.
func Subject(d Data) string {
	return fmt.Sprintf("%s — %s analytics", d.SiteName, d.PeriodLabel)
}

var funcs = template.FuncMap{
	"num":        formatNumber,
	"delta":      formatDelta,
	"deltaColor": deltaColor,
	"share":      sharePercent,
	"dict":       dict,
}

// dict builds a map from alternating key/value pairs.
//
// Go templates cannot pass more than one argument to a sub-template, and the
// metric and table blocks each need several. This is the standard workaround.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments (%d)", len(pairs))
	}

	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is not a string", pairs[i])
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}

// Render returns the report's HTML.
func Render(d Data) (string, error) {
	var out bytes.Buffer
	if err := reportTemplate.Execute(&out, d); err != nil {
		return "", fmt.Errorf("report: render: %w", err)
	}
	return out.String(), nil
}

// formatNumber groups thousands. No abbreviation: an email is read once and
// forwarded, and "1.2K" invites the question the exact number answers.
func formatNumber(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	return strings.Join(append([]string{s}, parts...), ",")
}

// formatDelta renders a change against the comparison window.
//
// The label is passed in because a month-to-date report compares against the
// same days of the previous month, not the whole of it.
func formatDelta(change *float64, against string) string {
	if against == "" {
		against = "last month"
	}
	if change == nil {
		// Nothing to compare against. Saying "0%" or "+100%" would both be
		// untrue.
		return "First month"
	}
	if *change > 0 {
		return fmt.Sprintf("▲ %.1f%% vs %s", *change, against)
	}
	if *change < 0 {
		return fmt.Sprintf("▼ %.1f%% vs %s", -*change, against)
	}
	return "Unchanged vs " + against
}

func deltaColor(change *float64) string {
	switch {
	case change == nil:
		return colorSubtle
	case *change > 0:
		return colorPositive
	case *change < 0:
		return colorNegative
	default:
		return colorMuted
	}
}

// sharePercent sizes a row's bar against the busiest row.
func sharePercent(value, max int64) int {
	if max <= 0 {
		return 0
	}
	pct := int(value * 100 / max)
	if pct < 1 {
		return 1
	}
	return pct
}

// reportTemplate is the monthly email.
//
// Tables, not divs, and inline styles, not classes: this has to render in
// Outlook, which uses Word's rendering engine and understands neither flexbox
// nor grid. The layout is deliberately one narrow column so it needs no
// responsive rules to survive a phone.
var reportTemplate = template.Must(template.New("report").Funcs(funcs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ .SiteName }} — {{ .PeriodLabel }}</title>
</head>
<body style="margin:0;padding:0;background:` + colorSurface + `;font-family:` + fontStack + `;color:` + colorText + `;">
  <!-- Shown in the inbox preview line, then hidden. -->
  <div style="display:none;max-height:0;overflow:hidden;opacity:0;">
    {{ num .Summary.Visitors }} visitors and {{ num .Summary.Pageviews }} pageviews in {{ .PeriodLabel }}.
  </div>

  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:` + colorSurface + `;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border:1px solid ` + colorBorder + `;border-radius:8px;">

          <tr>
            <td style="padding:32px 32px 24px;">
              <p style="margin:0 0 4px;font-size:13px;font-weight:500;letter-spacing:0.04em;text-transform:uppercase;color:` + colorMuted + `;">
                {{ .PeriodLabel }}
              </p>
              <h1 style="margin:0;font-size:24px;font-weight:600;line-height:1.2;color:` + colorText + `;">
                {{ .SiteName }}
              </h1>
              <p style="margin:4px 0 0;font-family:` + monoStack + `;font-size:13px;color:` + colorSubtle + `;">
                {{ .SiteDomain }}
              </p>
            </td>
          </tr>

          <!-- The three numbers the whole product reports. -->
          <tr>
            <td style="padding:0 32px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                {{ template "metric" dict "Label" "Unique visitors" "Value" .Summary.Visitors "Change" .Change.Visitors "Against" .CompareLabel }}
                {{ template "metric" dict "Label" "Pageviews" "Value" .Summary.Pageviews "Change" .Change.Pageviews "Against" .CompareLabel }}
                {{ template "metric" dict "Label" "Sessions" "Value" .Summary.Sessions "Change" .Change.Sessions "Against" .CompareLabel }}
              </table>
            </td>
          </tr>

          {{ if .TopPages }}
          <tr><td style="padding:8px 32px 0;">
            {{ template "table" dict "Title" "Top pages" "Rows" .TopPages "Metric" "pageviews" }}
          </td></tr>
          {{ end }}

          {{ if .TopReferrers }}
          <tr><td style="padding:8px 32px 0;">
            {{ template "table" dict "Title" "Top referrers" "Rows" .TopReferrers "Metric" "visitors" }}
          </td></tr>
          {{ end }}

          {{ if .TopCountries }}
          <tr><td style="padding:8px 32px 0;">
            {{ template "table" dict "Title" "Top countries" "Rows" .TopCountries "Metric" "visitors" }}
          </td></tr>
          {{ end }}

          {{ if .TopDevices }}
          <tr><td style="padding:8px 32px 0;">
            {{ template "table" dict "Title" "Devices" "Rows" .TopDevices "Metric" "visitors" }}
          </td></tr>
          {{ end }}

          {{ if not .Summary.Pageviews }}
          <tr><td style="padding:24px 32px;">
            <p style="margin:0;color:` + colorMuted + `;text-align:center;">
              No traffic recorded in {{ .PeriodLabel }}.
            </p>
          </td></tr>
          {{ end }}

          {{ if .DashboardURL }}
          <tr>
            <td style="padding:24px 32px 32px;" align="center">
              <a href="{{ .DashboardURL }}" style="display:inline-block;padding:12px 24px;background:` + colorAccent + `;color:#ffffff;font-size:15px;font-weight:500;text-decoration:none;border-radius:6px;">
                View full analytics
              </a>
            </td>
          </tr>
          {{ else }}
          <tr><td style="padding:16px;"></td></tr>
          {{ end }}

        </table>

        <p style="max-width:560px;margin:16px auto 0;font-size:12px;color:` + colorSubtle + `;text-align:center;line-height:1.5;">
          Sent by Zenith. Privacy-first analytics — no cookies, no tracking across sites.
        </p>
      </td>
    </tr>
  </table>
</body>
</html>

{{ define "metric" }}
<tr>
  <td style="padding:12px 0;border-bottom:1px solid ` + colorBorder + `;">
    <p style="margin:0;font-size:13px;font-weight:500;letter-spacing:0.04em;text-transform:uppercase;color:` + colorMuted + `;">
      {{ .Label }}
    </p>
    <p style="margin:4px 0 0;font-family:` + monoStack + `;font-size:32px;font-weight:600;line-height:1.2;color:` + colorText + `;">
      {{ num .Value }}
    </p>
    <p style="margin:4px 0 0;font-size:13px;color:{{ deltaColor .Change }};">
      {{ delta .Change .Against }}
    </p>
  </td>
</tr>
{{ end }}

{{ define "table" }}
<p style="margin:16px 0 8px;font-size:13px;font-weight:500;letter-spacing:0.04em;text-transform:uppercase;color:` + colorMuted + `;">
  {{ .Title }}
</p>
<table role="presentation" width="100%" cellpadding="0" cellspacing="0">
  {{ $max := 0 }}
  {{ range $i, $row := .Rows }}{{ if eq $i 0 }}{{ if eq $.Metric "pageviews" }}{{ $max = $row.Pageviews }}{{ else }}{{ $max = $row.Visitors }}{{ end }}{{ end }}{{ end }}
  {{ range .Rows }}
  {{ $value := .Visitors }}{{ if eq $.Metric "pageviews" }}{{ $value = .Pageviews }}{{ end }}
  <tr>
    <td style="padding:8px 0;border-bottom:1px solid ` + colorBorder + `;">
      <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
        <tr>
          <td style="font-size:14px;color:` + colorText + `;word-break:break-all;">{{ .Label }}</td>
          <td align="right" style="font-family:` + monoStack + `;font-size:14px;color:` + colorMuted + `;white-space:nowrap;padding-left:16px;">
            {{ num $value }}
          </td>
        </tr>
        <tr>
          <td colspan="2" style="padding-top:6px;">
            <!-- The bar is a table cell with a background: the only bar chart
                 an email client will draw. -->
            <table role="presentation" cellpadding="0" cellspacing="0" style="width:{{ share $value $max }}%;">
              <tr><td style="height:2px;background:` + colorAccent + `;opacity:0.25;font-size:0;line-height:0;">&nbsp;</td></tr>
            </table>
          </td>
        </tr>
      </table>
    </td>
  </tr>
  {{ end }}
</table>
{{ end }}
`))
