package report

import (
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
)

// regions renders region names in English.
//
// The report has one audience per site and no locale to read from an email, so
// English is the honest default rather than a guess.
var regions = display.English.Regions()

// countryName turns an ISO code into a name.
//
// Unrecognized codes are returned as-is: a row reading "ZZ" is odd, but it is
// better than an empty row or a dropped one.
func countryName(code string) string {
	if code == "" {
		return code
	}

	region, err := language.ParseRegion(code)
	if err != nil {
		return code
	}

	if name := regions.Name(region); name != "" {
		return name
	}
	return code
}
