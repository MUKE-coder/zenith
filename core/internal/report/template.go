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

	// Pill backgrounds. Flat hex rather than an alpha of the ink: Outlook
	// ignores rgba(), and a badge that loses its background there reads as
	// coloured text floating in the header.
	tintPositive = "#dcfce7"
	tintNegative = "#fee2e2"
	tintWarning  = "#fef3c7"

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
// reportTemplate is the monthly email.
//
// Tables, not divs, and inline styles, not classes: this has to render in
// Outlook, which has no flexbox, no grid and no <style> worth the name. Every
// width is fixed or a percentage, because an email client will not run the
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
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border:1px solid ` + colorBorder + `;border-radius:12px;">

          <tr>
            <td style="padding:28px 28px 20px;border-bottom:1px solid ` + colorBorder + `;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  <td>
                    <p style="margin:0 0 6px;font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:` + colorSubtle + `;">
                      Analytics · {{ .PeriodLabel }}
                    </p>
                    <h1 style="margin:0;font-size:24px;font-weight:600;line-height:1.2;color:` + colorText + `;">
                      {{ .SiteName }}
                    </h1>
                    <p style="margin:6px 0 0;font-family:` + monoStack + `;font-size:13px;">
                      <a href="https://{{ .SiteDomain }}" style="color:` + colorAccent + `;text-decoration:none;">{{ .SiteDomain }}</a>
                    </p>
                  </td>
                  {{ if not .Change.Visitors }}
                  <!-- No prior period to compare against, said once here rather
                       than three times under three tiles. -->
                  <td align="right" valign="top" style="white-space:nowrap;">
                    <span style="display:inline-block;padding:5px 10px;border-radius:999px;background:` + tintPositive + `;color:` + colorPositive + `;font-size:11px;font-weight:600;">
                      First month
                    </span>
                  </td>
                  {{ end }}
                </tr>
              </table>
            </td>
          </tr>

          <!-- The three numbers the whole product reports. -->
          <tr>
            <td style="padding:20px 28px 4px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  {{ template "tile" dict "Label" "Unique visitors" "Value" .Summary.Visitors "Change" .Change.Visitors "Against" .CompareLabel }}
                  <td width="10" style="font-size:0;line-height:0;">&nbsp;</td>
                  {{ template "tile" dict "Label" "Pageviews" "Value" .Summary.Pageviews "Change" .Change.Pageviews "Against" .CompareLabel }}
                  <td width="10" style="font-size:0;line-height:0;">&nbsp;</td>
                  {{ template "tile" dict "Label" "Sessions" "Value" .Summary.Sessions "Change" .Change.Sessions "Against" .CompareLabel }}
                </tr>
              </table>
            </td>
          </tr>

          {{ if .TopPages }}
          <tr><td style="padding:20px 28px 0;">
            {{ template "heading" "Top pages" }}
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
              {{ range .TopPages }}
              {{ template "row" dict "Label" .Label "Value" .Pageviews "Mono" true }}
              {{ end }}
            </table>
          </td></tr>
          {{ end }}

          {{ if or .TopReferrers .TopCountries }}
          <tr>
            <td style="padding:20px 28px 0;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  <td width="48%" valign="top">
                    {{ if .TopReferrers }}
                    {{ template "heading" "Top referrers" }}
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                      {{ range .TopReferrers }}
                      {{ template "row" dict "Label" .Label "Value" .Visitors "Link" true }}
                      {{ end }}
                    </table>
                    {{ end }}
                  </td>
                  <td width="4%" style="font-size:0;line-height:0;">&nbsp;</td>
                  <td width="48%" valign="top">
                    {{ if .TopCountries }}
                    {{ template "heading" "Top countries" }}
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                      {{ range .TopCountries }}
                      <tr>
                        <td style="padding:7px 0;border-bottom:1px solid ` + colorBorder + `;font-size:14px;color:` + colorText + `;">
                          <span style="font-family:` + monoStack + `;font-size:11px;color:` + colorSubtle + `;text-transform:uppercase;">{{ .Code }}</span>
                          &nbsp;{{ .Name }}
                        </td>
                        <td align="right" style="padding:7px 0;border-bottom:1px solid ` + colorBorder + `;font-family:` + monoStack + `;font-size:13px;color:` + colorMuted + `;white-space:nowrap;">
                          {{ num .Visitors }}
                        </td>
                      </tr>
                      {{ end }}
                    </table>
                    {{ end }}
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          {{ end }}

          {{ if .TopDevices }}
          <tr><td style="padding:20px 28px 0;">
            {{ template "heading" "Devices" }}
            <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
              {{ range .TopDevices }}
              {{ template "row" dict "Label" .Label "Value" .Visitors }}
              {{ end }}
            </table>
          </td></tr>
          {{ end }}

          {{ if not .Summary.Pageviews }}
          <tr><td style="padding:24px 28px;">
            <p style="margin:0;color:` + colorMuted + `;text-align:center;font-size:14px;">
              No traffic recorded in {{ .PeriodLabel }}.
            </p>
          </td></tr>
          {{ end }}

          {{ if .DashboardURL }}
          <tr>
            <td style="padding:24px 28px 28px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  <td align="center" style="background:` + colorText + `;border-radius:8px;">
                    <a href="{{ .DashboardURL }}" style="display:block;padding:14px 24px;color:#ffffff;font-size:15px;font-weight:500;text-decoration:none;">
                      View full dashboard →
                    </a>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          {{ else }}
          <tr><td style="padding:14px;"></td></tr>
          {{ end }}

        </table>

        <p style="max-width:560px;margin:16px auto 0;font-size:12px;color:` + colorSubtle + `;text-align:center;line-height:1.6;">
          Sent by {{ with .SentBy }}{{ . }}{{ else }}Zenith{{ end }} · {{ .SiteName }} Analytics<br>
          You&rsquo;re receiving this because you manage this site.
        </p>
      </td>
    </tr>
  </table>
</body>
</html>

{{ define "heading" }}
<p style="margin:0 0 4px;font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:` + colorSubtle + `;">
  {{ . }}
</p>
{{ end }}

{{ define "tile" }}
<td width="33%" valign="top" style="border:1px solid ` + colorBorder + `;border-radius:8px;padding:14px 12px;">
  <p style="margin:0;font-size:10px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;color:` + colorSubtle + `;">
    {{ .Label }}
  </p>
  <p style="margin:6px 0 0;font-family:` + monoStack + `;font-size:26px;font-weight:600;line-height:1.1;color:` + colorText + `;">
    {{ num .Value }}
  </p>
  {{ if .Change }}
  <p style="margin:4px 0 0;font-size:11px;color:{{ deltaColor .Change }};">
    {{ delta .Change .Against }}
  </p>
  {{ end }}
</td>
{{ end }}

{{ define "row" }}
<tr>
  <td style="padding:7px 0;border-bottom:1px solid ` + colorBorder + `;font-size:14px;color:{{ if .Link }}` + colorAccent + `{{ else }}` + colorText + `{{ end }};word-break:break-all;{{ if .Mono }}font-family:` + monoStack + `;font-size:13px;{{ end }}">
    {{ .Label }}
  </td>
  <td align="right" style="padding:7px 0;border-bottom:1px solid ` + colorBorder + `;font-family:` + monoStack + `;font-size:13px;color:` + colorMuted + `;white-space:nowrap;padding-left:12px;">
    {{ num .Value }}
  </td>
</tr>
{{ end }}
`))
