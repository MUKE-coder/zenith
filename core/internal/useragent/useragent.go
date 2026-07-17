// Package useragent classifies a user-agent string into coarse buckets.
//
// Coarse is the point. We keep "desktop / Firefox / Linux", never the raw
// string: full user agents are high-entropy enough to help fingerprint an
// individual, which is exactly what Zenith promises not to do. The raw string
// is used to derive the daily visitor hash and then dropped.
package useragent

import (
	"strings"

	ua "github.com/mileusna/useragent"
)

// automationTools are clients that are plainly not people.
//
// The parser recognizes declared crawlers (Googlebot) and headless browsers,
// but reports HTTP libraries as ordinary clients -- a curl request would
// otherwise be counted as a visitor and quietly inflate the numbers. Matching
// the parsed client name rather than searching the raw string for "bot" is
// deliberate: "bot" appears inside real device names, Cubot phones among them,
// and discarding those visitors would be a worse error than counting a script.
var automationTools = map[string]bool{
	"curl":              true,
	"wget":              true,
	"python-requests":   true,
	"python-urllib":     true,
	"go-http-client":    true,
	"java":              true,
	"okhttp":            true,
	"axios":             true,
	"node-fetch":        true,
	"got":               true,
	"postmanruntime":    true,
	"insomnia":          true,
	"httpie":            true,
	"libwww-perl":       true,
	"guzzlehttp":        true,
	"apache-httpclient": true,
	"restsharp":         true,
	"scrapy":            true,
	"phantomjs":         true,
	"lighthouse":        true,
	"headlesschrome":    true,
	"headless chrome":   true,
}

// Device buckets.
const (
	DeviceDesktop = "desktop"
	DeviceMobile  = "mobile"
	DeviceTablet  = "tablet"
	DeviceBot     = "bot"
	DeviceUnknown = "unknown"
)

// Unknown is what every field is when the user agent tells us nothing.
const Unknown = "unknown"

// Result is the coarse classification of one user agent.
type Result struct {
	Device  string
	Browser string
	OS      string

	// Bot reports an automated client. Bots are not visitors, and counting
	// them would quietly inflate every number on the dashboard.
	Bot bool
}

// Parse classifies a raw user-agent string.
func Parse(raw string) Result {
	if raw == "" {
		return Result{Device: DeviceUnknown, Browser: Unknown, OS: Unknown}
	}

	parsed := ua.Parse(raw)

	if parsed.Bot || automationTools[strings.ToLower(parsed.Name)] {
		return Result{
			Device:  DeviceBot,
			Browser: orUnknown(parsed.Name),
			OS:      orUnknown(parsed.OS),
			Bot:     true,
		}
	}

	return Result{
		Device:  device(parsed),
		Browser: orUnknown(parsed.Name),
		OS:      orUnknown(parsed.OS),
	}
}

func device(parsed ua.UserAgent) string {
	switch {
	case parsed.Tablet:
		return DeviceTablet
	case parsed.Mobile:
		return DeviceMobile
	case parsed.Desktop:
		return DeviceDesktop
	default:
		return DeviceUnknown
	}
}

func orUnknown(v string) string {
	if v == "" {
		return Unknown
	}
	return v
}
