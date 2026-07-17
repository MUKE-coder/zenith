package audit_test

import (
	"testing"

	"github.com/zenith/audit-worker/internal/audit"
)

// checkFor returns the structured-data finding for a set of blocks.
func jsonldCheck(t *testing.T, blocks ...string) audit.Check {
	t.Helper()

	page := good()
	page.JSONLD = blocks

	for _, check := range audit.Evaluate(page, nil).Checks {
		switch check.ID {
		case "jsonld.ok", "jsonld.missing", "jsonld.invalid":
			return check
		}
	}
	t.Fatal("no structured data check")
	return audit.Check{}
}

func TestValidJSONLD(t *testing.T) {
	valid := []string{
		`{"@context":"https://schema.org","@type":"Organization","name":"Acme"}`,
		`[{"@context":"https://schema.org","@type":"Person","name":"A"}]`,
		// A @graph carries its context on the wrapper.
		`{"@context":"https://schema.org","@graph":[{"@type":"WebSite","name":"X"}]}`,
	}

	for _, block := range valid {
		check := jsonldCheck(t, block)
		if check.ID != "jsonld.ok" {
			t.Errorf("%s\n  was rejected: %s", block, check.Message)
		}
	}
}

// Both failures are silent: the page looks fine and the rich result simply
// never appears. That is why they are worth naming.
func TestInvalidJSONLD(t *testing.T) {
	cases := map[string]string{
		"not json":       `{ this is not valid json }`,
		"no @context":    `{"@type":"Organization","name":"Acme"}`,
		"no @type":       `{"@context":"https://schema.org","name":"Acme"}`,
		"empty array":    `[]`,
		"empty graph":    `{"@context":"https://schema.org","@graph":[]}`,
		"graph item bad": `{"@context":"https://schema.org","@graph":[{"name":"no type"}]}`,
		"a bare string":  `"hello"`,
		"array of junk":  `["not an object"]`,
	}

	for name, block := range cases {
		t.Run(name, func(t *testing.T) {
			check := jsonldCheck(t, block)
			if check.ID != "jsonld.invalid" {
				t.Errorf("accepted %s: got %q", block, check.ID)
			}
			if check.Severity != audit.SeverityError {
				t.Errorf("severity = %q, want error", check.Severity)
			}
		})
	}
}

func TestNoJSONLDIsAWarning(t *testing.T) {
	check := jsonldCheck(t)

	if check.ID != "jsonld.missing" {
		t.Errorf("id = %q, want jsonld.missing", check.ID)
	}
	// A warning, not an error: plenty of good pages have no structured data.
	if check.Severity != audit.SeverityWarning {
		t.Errorf("severity = %q, want warning", check.Severity)
	}
}

// One bad block among good ones must still be caught.
func TestOneBadBlockAmongGoodOnes(t *testing.T) {
	check := jsonldCheck(t,
		`{"@context":"https://schema.org","@type":"Organization","name":"Acme"}`,
		`{ broken }`,
	)

	if check.ID != "jsonld.invalid" {
		t.Errorf("id = %q, want jsonld.invalid", check.ID)
	}
}
