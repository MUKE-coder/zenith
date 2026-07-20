package report

import (
	"bytes"
	"fmt"
	"html/template"
)

// RenderSEO returns the SEO report's HTML.
func RenderSEO(d SEOData) (string, error) {
	var out bytes.Buffer
	if err := seoTemplate.Execute(&out, d); err != nil {
		return "", fmt.Errorf("report: render seo: %w", err)
	}
	return out.String(), nil
}

var seoFuncs = template.FuncMap{
	"severityLabel":  severityLabel,
	"severityColor":  severityColor,
	"severityTint":   severityTint,
	"scoreTint":      scoreTint,
	"pageScoreColor": scoreColor,
	"scoreBar":       scoreBar,
	// Shared with the analytics email: Go templates pass one argument to a
	// sub-template, and the tile block needs three.
	"dict": dict,
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
}

// scoreBar clamps a score to a width the meter can draw. A zero-width bar
// looks like a broken image, so the floor is visible rather than absent.
func scoreBar(score int) int {
	switch {
	case score < 2:
		return 2
	case score > 100:
		return 100
	default:
		return score
	}
}

// The SEO report. Same construction rules as the analytics one: tables, inline
// styles, no external images -- see the note on reportTemplate.
var seoTemplate = template.Must(template.New("seo").Funcs(seoFuncs).Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{ .SiteName }} — SEO audit</title>
</head>
<body style="margin:0;padding:0;background:` + colorSurface + `;font-family:` + fontStack + `;color:` + colorText + `;">
  <div style="display:none;max-height:0;overflow:hidden;opacity:0;">
    Score {{ .Score }}/100 across {{ .PagesAudited }} {{ plural .PagesAudited "page" "pages" }}.
  </div>

  <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:` + colorSurface + `;padding:32px 16px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border:1px solid ` + colorBorder + `;border-radius:12px;">

          <tr>
            <td style="padding:28px 28px 20px;border-bottom:1px solid ` + colorBorder + `;">
              <p style="margin:0 0 6px;font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:` + colorSubtle + `;">
                SEO audit{{ if .AuditedLabel }} · {{ .AuditedLabel }}{{ end }}
              </p>
              <h1 style="margin:0;font-size:24px;font-weight:600;line-height:1.2;color:` + colorText + `;">
                {{ .SiteName }}
              </h1>
              <p style="margin:6px 0 0;font-family:` + monoStack + `;font-size:13px;">
                <a href="https://{{ .SiteDomain }}" style="color:` + colorAccent + `;text-decoration:none;">{{ .SiteDomain }}</a>
              </p>
            </td>
          </tr>

          <!-- The score, which is the one number this email exists to deliver. -->
          <tr>
            <td style="padding:20px 28px 0;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorBorder + `;border-radius:8px;">
                <tr>
                  <td style="padding:18px 20px;">
                    <p style="margin:0 0 8px;font-size:10px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;color:` + colorSubtle + `;">
                      Overall score
                    </p>
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                      <tr>
                        <td valign="middle">
                          <span style="font-family:` + monoStack + `;font-size:38px;font-weight:600;line-height:1;color:{{ .ScoreColor }};">{{ .Score }}</span><span style="font-family:` + monoStack + `;font-size:16px;color:` + colorSubtle + `;">/100</span>
                        </td>
                        <td valign="middle" align="right" width="45%">
                          <!-- The meter: a filled cell inside an empty one, which
                               is as close to a progress bar as email gets. -->
                          <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background:` + colorBorder + `;border-radius:999px;">
                            <tr>
                              <td style="font-size:0;line-height:0;">
                                <table role="presentation" width="{{ scoreBar .Score }}%" cellpadding="0" cellspacing="0" style="border-radius:999px;">
                                  <tr><td style="height:6px;background:{{ .ScoreColor }};font-size:0;line-height:0;border-radius:999px;">&nbsp;</td></tr>
                                </table>
                              </td>
                            </tr>
                          </table>
                        </td>
                      </tr>
                    </table>
                    <p style="margin:12px 0 0;">
                      <span style="display:inline-block;padding:4px 10px;border-radius:999px;background:{{ scoreTint .Score }};color:{{ .ScoreColor }};font-size:11px;font-weight:600;">
                        {{ .ScoreLabel }}
                      </span>
                      <span style="font-size:13px;color:` + colorMuted + `;">
                        &nbsp;Across {{ .PagesAudited }} {{ plural .PagesAudited "page" "pages" }}
                      </span>
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          {{ if .Issues }}
          <tr>
            <td style="padding:20px 28px 0;">
              <p style="margin:0 0 10px;font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:` + colorSubtle + `;">
                What to fix first
              </p>

              {{ range .Issues }}
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorBorder + `;border-radius:8px;margin-bottom:8px;">
                <tr>
                  <td style="padding:14px 16px;">
                    <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                      <tr>
                        <td>
                          <span style="display:inline-block;padding:3px 8px;border-radius:999px;background:{{ severityTint .Severity }};color:{{ severityColor .Severity }};font-size:10px;font-weight:700;letter-spacing:0.04em;text-transform:uppercase;">
                            {{ severityLabel .Severity }}
                          </span>
                        </td>
                        <td align="right" style="font-size:11px;color:` + colorSubtle + `;white-space:nowrap;">
                          {{ .Pages }} {{ plural .Pages "page" "pages" }} affected
                        </td>
                      </tr>
                    </table>
                    <p style="margin:10px 0 0;font-size:15px;font-weight:600;line-height:1.35;color:` + colorText + `;">
                      {{ .Title }}
                    </p>
                    {{ if .Detail }}
                    <p style="margin:4px 0 0;font-size:13px;line-height:1.45;color:` + colorMuted + `;">
                      {{ .Detail }}
                    </p>
                    {{ end }}
                    <p style="margin:6px 0 0;font-family:` + monoStack + `;font-size:12px;color:` + colorSubtle + `;word-break:break-all;">
                      e.g. {{ .Example }}
                    </p>
                  </td>
                </tr>
              </table>
              {{ end }}
            </td>
          </tr>

          <!-- The totals, because the list above is only where to start. -->
          <tr>
            <td style="padding:12px 28px 0;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  {{ template "seoTile" dict "Value" .Errors "Label" (plural .Errors "Error" "Errors") "Color" "` + colorNegative + `" }}
                  <td width="10" style="font-size:0;line-height:0;">&nbsp;</td>
                  {{ template "seoTile" dict "Value" .Warnings "Label" (plural .Warnings "Warning" "Warnings") "Color" "` + colorWarning + `" }}
                  <td width="10" style="font-size:0;line-height:0;">&nbsp;</td>
                  {{ template "seoTile" dict "Value" .PagesAudited "Label" (plural .PagesAudited "Page" "Pages") "Color" "` + colorText + `" }}
                </tr>
              </table>
            </td>
          </tr>
          {{ else }}
          <tr><td style="padding:24px 28px 0;">
            <p style="margin:0;color:` + colorPositive + `;text-align:center;font-size:15px;">
              No errors or warnings found. Nothing to fix.
            </p>
          </td></tr>
          {{ end }}

          {{ if .Pages }}
          <tr>
            <td style="padding:20px 28px 0;">
              <p style="margin:0 0 4px;font-size:11px;font-weight:600;letter-spacing:0.08em;text-transform:uppercase;color:` + colorSubtle + `;">
                Lowest scoring pages
              </p>
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                {{ range .Pages }}
                <tr>
                  <td style="padding:7px 0;border-bottom:1px solid ` + colorBorder + `;font-family:` + monoStack + `;font-size:13px;color:` + colorMuted + `;word-break:break-all;">
                    {{ .Path }}
                  </td>
                  <td width="48" align="right" style="padding:7px 0;border-bottom:1px solid ` + colorBorder + `;font-family:` + monoStack + `;font-size:13px;font-weight:600;color:{{ pageScoreColor .Score }};">
                    {{ .Score }}
                  </td>
                </tr>
                {{ end }}
              </table>
            </td>
          </tr>
          {{ end }}

          {{ if .DashboardURL }}
          <tr>
            <td style="padding:24px 28px 28px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                <tr>
                  <td align="center" style="background:` + colorText + `;border-radius:8px;">
                    <a href="{{ .DashboardURL }}" style="display:block;padding:14px 24px;color:#ffffff;font-size:15px;font-weight:500;text-decoration:none;">
                      View full report →
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
          Powered by <a href="` + zenithURL + `" style="color:` + colorMuted + `;text-decoration:underline;">Zenith</a> · {{ .SiteName }} SEO audit<br>
          You&rsquo;re receiving this because you manage this site.
        </p>
      </td>
    </tr>
  </table>
</body>
</html>

{{ define "seoTile" }}
<td width="33%" align="center" valign="top" style="border:1px solid ` + colorBorder + `;border-radius:8px;padding:12px 8px;">
  <p style="margin:0;font-family:` + monoStack + `;font-size:22px;font-weight:600;line-height:1.1;color:{{ .Color }};">
    {{ .Value }}
  </p>
  <p style="margin:4px 0 0;font-size:10px;font-weight:600;letter-spacing:0.06em;text-transform:uppercase;color:` + colorSubtle + `;">
    {{ .Label }}
  </p>
</td>
{{ end }}
`))
