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
	"pageScoreColor": scoreColor,
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
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
        <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="max-width:560px;background:#ffffff;border:1px solid ` + colorBorder + `;border-radius:8px;">

          <tr>
            <td style="padding:32px 32px 24px;">
              <p style="margin:0 0 4px;font-size:13px;font-weight:500;letter-spacing:0.04em;text-transform:uppercase;color:` + colorMuted + `;">
                SEO audit{{ if .AuditedLabel }} — {{ .AuditedLabel }}{{ end }}
              </p>
              <h1 style="margin:0;font-size:24px;font-weight:600;line-height:1.2;color:` + colorText + `;">
                {{ .SiteName }}
              </h1>
              <p style="margin:4px 0 0;font-family:` + monoStack + `;font-size:13px;color:` + colorSubtle + `;">
                {{ .SiteDomain }}
              </p>
            </td>
          </tr>

          <!-- The score, which is the one number this email exists to deliver. -->
          <tr>
            <td style="padding:0 32px;">
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="border:1px solid ` + colorBorder + `;border-radius:8px;">
                <tr>
                  <td style="padding:24px;" align="center">
                    <p style="margin:0;font-family:` + monoStack + `;font-size:44px;font-weight:600;line-height:1;color:{{ .ScoreColor }};">
                      {{ .Score }}<span style="font-size:20px;color:` + colorSubtle + `;">/100</span>
                    </p>
                    <p style="margin:8px 0 0;font-size:15px;font-weight:500;color:{{ .ScoreColor }};">
                      {{ .ScoreLabel }}
                    </p>
                    <p style="margin:4px 0 0;font-size:13px;color:` + colorMuted + `;">
                      Across {{ .PagesAudited }} {{ plural .PagesAudited "page" "pages" }}
                    </p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>

          {{ if .Issues }}
          <tr>
            <td style="padding:24px 32px 0;">
              <p style="margin:0 0 8px;font-size:13px;font-weight:500;letter-spacing:0.04em;text-transform:uppercase;color:` + colorMuted + `;">
                What to fix first
              </p>
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                {{ range .Issues }}
                <tr>
                  <td style="padding:12px 0;border-bottom:1px solid ` + colorBorder + `;">
                    <p style="margin:0;font-size:12px;font-weight:600;letter-spacing:0.03em;text-transform:uppercase;color:{{ severityColor .Severity }};">
                      {{ severityLabel .Severity }}
                    </p>
                    <p style="margin:4px 0 0;font-size:15px;line-height:1.45;color:` + colorText + `;">
                      {{ .Message }}
                    </p>
                    <p style="margin:4px 0 0;font-family:` + monoStack + `;font-size:12px;color:` + colorSubtle + `;">
                      {{ .Pages }} {{ plural .Pages "page" "pages" }} · e.g. {{ .Example }}
                    </p>
                  </td>
                </tr>
                {{ end }}
              </table>
            </td>
          </tr>
          {{ else }}
          <tr><td style="padding:24px 32px 0;">
            <p style="margin:0;color:` + colorPositive + `;text-align:center;font-size:15px;">
              No errors or warnings found. Nothing to fix.
            </p>
          </td></tr>
          {{ end }}

          {{ if .Pages }}
          <tr>
            <td style="padding:24px 32px 0;">
              <p style="margin:0 0 8px;font-size:13px;font-weight:500;letter-spacing:0.04em;text-transform:uppercase;color:` + colorMuted + `;">
                Lowest scoring pages
              </p>
              <table role="presentation" width="100%" cellpadding="0" cellspacing="0">
                {{ range .Pages }}
                <tr>
                  <td style="padding:8px 0;border-bottom:1px solid ` + colorBorder + `;font-family:` + monoStack + `;font-size:13px;color:` + colorMuted + `;">
                    {{ .Path }}
                  </td>
                  <td width="56" align="right" style="padding:8px 0;border-bottom:1px solid ` + colorBorder + `;font-family:` + monoStack + `;font-size:13px;font-weight:600;color:{{ pageScoreColor .Score }};">
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
            <td style="padding:24px 32px 32px;" align="center">
              <a href="{{ .DashboardURL }}" style="display:inline-block;padding:12px 24px;background:` + colorAccent + `;color:#ffffff;font-size:15px;font-weight:500;text-decoration:none;border-radius:6px;">
                View the full audit
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
`))
