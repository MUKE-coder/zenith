package useragent_test

import (
	"testing"

	"github.com/zenith/core/internal/useragent"
)

func TestParseRealAgents(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		device  string
		browser string
		os      string
	}{
		{
			name:    "firefox on linux",
			raw:     "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0",
			device:  useragent.DeviceDesktop,
			browser: "Firefox",
			os:      "Linux",
		},
		{
			name:    "chrome on windows",
			raw:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			device:  useragent.DeviceDesktop,
			browser: "Chrome",
			os:      "Windows",
		},
		{
			name:    "safari on iphone",
			raw:     "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			device:  useragent.DeviceMobile,
			browser: "Safari",
			os:      "iOS",
		},
		{
			name:    "safari on ipad",
			raw:     "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			device:  useragent.DeviceTablet,
			browser: "Safari",
			os:      "iOS",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := useragent.Parse(c.raw)

			if got.Device != c.device {
				t.Errorf("device = %q, want %q", got.Device, c.device)
			}
			if got.Browser != c.browser {
				t.Errorf("browser = %q, want %q", got.Browser, c.browser)
			}
			if got.OS != c.os {
				t.Errorf("os = %q, want %q", got.OS, c.os)
			}
			if got.Bot {
				t.Error("a real browser was classified as a bot")
			}
		})
	}
}

// Bots must be recognized: counting crawlers would inflate every number on
// every dashboard.
func TestParseDetectsBots(t *testing.T) {
	bots := []string{
		// Declared crawlers, which the parser knows.
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)",
		"Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)",
		// Headless browsers: puppeteer, Lighthouse, CI.
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36",
		// HTTP libraries, which the parser reports as ordinary clients.
		"curl/8.4.0",
		"Wget/1.21.3",
		"python-requests/2.31.0",
		"Go-http-client/1.1",
		"axios/1.6.0",
		"node-fetch/1.0",
		"PostmanRuntime/7.36.0",
	}

	for _, raw := range bots {
		got := useragent.Parse(raw)
		if !got.Bot {
			t.Errorf("Parse(%q).Bot = false, want true", raw)
		}
		if got.Device != useragent.DeviceBot {
			t.Errorf("Parse(%q).Device = %q, want %q", raw, got.Device, useragent.DeviceBot)
		}
	}
}

// Real devices whose names contain "bot" must still count as visitors. This is
// why detection matches the parsed client name instead of searching the raw
// string: Cubot is a phone brand, and dropping those visitors would be a worse
// error than counting the occasional script.
func TestParseDoesNotMistakeDevicesForBots(t *testing.T) {
	people := []string{
		"Mozilla/5.0 (Linux; Android 11; CUBOT X30) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36",
		"Mozilla/5.0 (Linux; Android 10; Cubot Note 20) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.181 Mobile Safari/537.36",
	}

	for _, raw := range people {
		got := useragent.Parse(raw)
		if got.Bot {
			t.Errorf("Parse(%q).Bot = true: a real phone was discarded as a bot", raw)
		}
		if got.Device != useragent.DeviceMobile {
			t.Errorf("Parse(%q).Device = %q, want mobile", raw, got.Device)
		}
	}
}

// An absent or junk user agent must not produce empty columns.
func TestParseHandlesMissingAndGarbage(t *testing.T) {
	for _, raw := range []string{"", "garbage", "-", "%%%"} {
		got := useragent.Parse(raw)

		if got.Device == "" || got.Browser == "" || got.OS == "" {
			t.Errorf("Parse(%q) left a field empty: %+v", raw, got)
		}
	}
}

func TestParseEmptyIsUnknown(t *testing.T) {
	got := useragent.Parse("")

	if got.Device != useragent.DeviceUnknown {
		t.Errorf("device = %q, want %q", got.Device, useragent.DeviceUnknown)
	}
	if got.Browser != useragent.Unknown || got.OS != useragent.Unknown {
		t.Errorf("browser/os = %q/%q, want unknown", got.Browser, got.OS)
	}
	if got.Bot {
		t.Error("an empty user agent was classified as a bot")
	}
}

// The classification is coarse on purpose: no version numbers, nothing with
// enough entropy to help fingerprint a person.
func TestParseKeepsClassificationCoarse(t *testing.T) {
	got := useragent.Parse("Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0")

	if got.Browser != "Firefox" {
		t.Errorf("browser = %q: it should be the family alone, with no version", got.Browser)
	}
	if got.OS != "Linux" {
		t.Errorf("os = %q: it should be the family alone, with no version", got.OS)
	}
}
