package http

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zenith/core/internal/storage"
)

// defaultRange is the period a stats request covers when it names none.
const defaultRange = 30 * 24 * time.Hour

// maxBreakdownLimit caps ?limit. The store caps it again; this one exists to
// reject an absurd request before it reaches SQL.
const maxBreakdownLimit = 500

// resolveSite returns the site this request is allowed to read.
//
// This is the boundary the whole product rests on: a developer sees every site,
// an owner sees exactly one, and the one an owner sees is named by their token
// rather than by the URL. Because the site id comes from the signed claim, an
// owner cannot ask for another client's site by editing a query parameter --
// there is no code path where their `?site=` becomes the id we query.
func (s *Server) resolveSite(w http.ResponseWriter, r *http.Request) (storage.Site, bool) {
	claims, ok := ClaimsFrom(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "Sign in first.")
		return storage.Site{}, false
	}

	requested := strings.TrimSpace(r.URL.Query().Get("site"))

	var siteID string
	switch claims.Role {
	case storage.RoleOwner:
		siteID = claims.SiteID
		// Naming a different site is not a mistake worth accommodating.
		if requested != "" && requested != claims.SiteID {
			writeError(w, http.StatusForbidden, "You don't have access to that site.")
			return storage.Site{}, false
		}

	case storage.RoleDeveloper:
		if requested == "" {
			writeError(w, http.StatusBadRequest, "Name a site with ?site=<id>.")
			return storage.Site{}, false
		}
		siteID = requested

	default:
		writeError(w, http.StatusForbidden, "You don't have access to that.")
		return storage.Site{}, false
	}

	site, err := s.app.SiteByID(r.Context(), siteID)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "No such site.")
		return storage.Site{}, false
	}
	if err != nil {
		s.log.Error("stats: site lookup", "err", err)
		writeError(w, http.StatusInternalServerError, "Something went wrong.")
		return storage.Site{}, false
	}

	return site, true
}

// buildQuery resolves the site and time range for a stats request.
func (s *Server) buildQuery(w http.ResponseWriter, r *http.Request) (storage.Query, bool) {
	site, ok := s.resolveSite(w, r)
	if !ok {
		return storage.Query{}, false
	}

	from, to, err := parseRange(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return storage.Query{}, false
	}

	limit, err := parseLimit(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return storage.Query{}, false
	}

	return storage.Query{
		SiteID: site.ID,
		From:   from,
		To:     to,
		Limit:  limit,
		// Internal navigation sets a referrer, so without this a site's top
		// referrer is always itself.
		ExcludeReferrer: strings.TrimPrefix(site.Domain, "www."),
	}, true
}

// previous returns the equal-length period immediately before q.
//
// Because ranges are half-open, the previous period ends exactly where this
// one begins: the two tile with no event counted twice or missed between.
func previous(q storage.Query) storage.Query {
	length := q.To.Sub(q.From)

	prev := q
	prev.To = q.From
	prev.From = q.From.Add(-length)
	return prev
}

type summaryResponse struct {
	Pageviews int64 `json:"pageviews"`
	Visitors  int64 `json:"visitors"`
	Sessions  int64 `json:"sessions"`

	// Previous and Change are present only when ?compare=true.
	Previous *summaryNumbers `json:"previous,omitempty"`
	Change   *summaryChange  `json:"change,omitempty"`
}

type summaryNumbers struct {
	Pageviews int64 `json:"pageviews"`
	Visitors  int64 `json:"visitors"`
	Sessions  int64 `json:"sessions"`
}

// summaryChange is the percent change against the previous period.
//
// Each field is nullable because "up from zero" has no percentage. Reporting
// 0% or 100% there would both be lies; null lets the interface say something
// true, like "new".
type summaryChange struct {
	Pageviews *float64 `json:"pageviews"`
	Visitors  *float64 `json:"visitors"`
	Sessions  *float64 `json:"sessions"`
}

// handleSummary returns the headline numbers for a period.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	current, err := s.events.Summary(r.Context(), q)
	if err != nil {
		s.log.Error("stats: summary", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load those stats. Try again.")
		return
	}

	out := summaryResponse{
		Pageviews: current.Pageviews,
		Visitors:  current.Visitors,
		Sessions:  current.Sessions,
	}

	if wantsCompare(r) {
		prior, err := s.events.Summary(r.Context(), previous(q))
		if err != nil {
			s.log.Error("stats: summary comparison", "err", err)
			writeError(w, http.StatusInternalServerError, "Couldn't load those stats. Try again.")
			return
		}

		out.Previous = &summaryNumbers{
			Pageviews: prior.Pageviews,
			Visitors:  prior.Visitors,
			Sessions:  prior.Sessions,
		}
		out.Change = &summaryChange{
			Pageviews: percentChange(prior.Pageviews, current.Pageviews),
			Visitors:  percentChange(prior.Visitors, current.Visitors),
			Sessions:  percentChange(prior.Sessions, current.Sessions),
		}
	}

	writeJSON(w, http.StatusOK, out)
}

// percentChange returns the change from before to now, or nil if there is no
// meaningful percentage to state.
func percentChange(before, now int64) *float64 {
	// Growth from nothing is not a percentage. The honest answer is "no
	// comparison", not 0% and not +100%.
	if before == 0 {
		return nil
	}

	change := (float64(now) - float64(before)) / float64(before) * 100

	// One decimal is all a delta chip shows; more would imply precision the
	// number does not have.
	rounded := float64(int64(change*10)) / 10
	return &rounded
}

type timeseriesResponse struct {
	Granularity storage.Granularity `json:"granularity"`
	Buckets     []bucketJSON        `json:"buckets"`
	Previous    []bucketJSON        `json:"previous,omitempty"`
}

type bucketJSON struct {
	TS        time.Time `json:"ts"`
	Pageviews int64     `json:"pageviews"`
	Visitors  int64     `json:"visitors"`
}

// handleTimeseries returns traffic over time.
func (s *Server) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	granularity := storage.Granularity(strings.TrimSpace(r.URL.Query().Get("granularity")))
	if granularity == "" {
		granularity = defaultGranularity(q)
	}
	if !granularity.Valid() {
		writeError(w, http.StatusBadRequest, "Granularity must be hour, day, week, or month.")
		return
	}

	buckets, err := s.events.Timeseries(r.Context(), q, granularity)
	if err != nil {
		s.log.Error("stats: timeseries", "err", err)
		writeError(w, http.StatusInternalServerError, "Couldn't load those stats. Try again.")
		return
	}

	out := timeseriesResponse{Granularity: granularity, Buckets: toBucketJSON(buckets)}

	if wantsCompare(r) {
		prior, err := s.events.Timeseries(r.Context(), previous(q), granularity)
		if err != nil {
			s.log.Error("stats: timeseries comparison", "err", err)
			writeError(w, http.StatusInternalServerError, "Couldn't load those stats. Try again.")
			return
		}
		out.Previous = toBucketJSON(prior)
	}

	writeJSON(w, http.StatusOK, out)
}

// defaultGranularity picks a bucket size that yields a readable chart.
//
// A year of hourly buckets is 8,760 points on a 600px chart: unreadable, and
// expensive to produce. The default scales with the range so the chart has
// roughly a screen's worth of points.
func defaultGranularity(q storage.Query) storage.Granularity {
	span := q.To.Sub(q.From)

	switch {
	case span <= 48*time.Hour:
		return storage.GranularityHour
	case span <= 90*24*time.Hour:
		return storage.GranularityDay
	case span <= 730*24*time.Hour:
		return storage.GranularityWeek
	default:
		return storage.GranularityMonth
	}
}

func toBucketJSON(buckets []storage.Bucket) []bucketJSON {
	out := make([]bucketJSON, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, bucketJSON{TS: b.TS, Pageviews: b.Pageviews, Visitors: b.Visitors})
	}
	return out
}

type pagesResponse struct {
	Top   []countJSON `json:"top"`
	Entry []countJSON `json:"entry"`
	Exit  []countJSON `json:"exit"`
}

type countJSON struct {
	Label     string `json:"label"`
	Visitors  int64  `json:"visitors"`
	Pageviews int64  `json:"pageviews"`
}

// handlePages returns top, entry, and exit pages.
func (s *Server) handlePages(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	top, err := s.events.Breakdown(r.Context(), q, storage.DimPath)
	if err != nil {
		s.statsError(w, "pages: top", err)
		return
	}
	entry, err := s.events.EntryPages(r.Context(), q)
	if err != nil {
		s.statsError(w, "pages: entry", err)
		return
	}
	exit, err := s.events.ExitPages(r.Context(), q)
	if err != nil {
		s.statsError(w, "pages: exit", err)
		return
	}

	writeJSON(w, http.StatusOK, pagesResponse{
		Top:   toCountJSON(top),
		Entry: toCountJSON(entry),
		Exit:  toCountJSON(exit),
	})
}

type referrersResponse struct {
	Sources []countJSON `json:"sources"`
	UTM     utmResponse `json:"utm"`
}

type utmResponse struct {
	Source   []countJSON `json:"source"`
	Medium   []countJSON `json:"medium"`
	Campaign []countJSON `json:"campaign"`
	Term     []countJSON `json:"term"`
	Content  []countJSON `json:"content"`
}

// handleReferrers returns referral sources and the UTM campaign breakdown.
func (s *Server) handleReferrers(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	dimensions := map[storage.Dimension][]countJSON{}
	for _, d := range []storage.Dimension{
		storage.DimReferrer, storage.DimUTMSource, storage.DimUTMMedium,
		storage.DimUTMCampaign, storage.DimUTMTerm, storage.DimUTMContent,
	} {
		rows, err := s.events.Breakdown(r.Context(), q, d)
		if err != nil {
			s.statsError(w, "referrers: "+string(d), err)
			return
		}
		dimensions[d] = toCountJSON(rows)
	}

	writeJSON(w, http.StatusOK, referrersResponse{
		Sources: dimensions[storage.DimReferrer],
		UTM: utmResponse{
			Source:   dimensions[storage.DimUTMSource],
			Medium:   dimensions[storage.DimUTMMedium],
			Campaign: dimensions[storage.DimUTMCampaign],
			Term:     dimensions[storage.DimUTMTerm],
			Content:  dimensions[storage.DimUTMContent],
		},
	})
}

type geoResponse struct {
	Countries []countJSON `json:"countries"`
}

// handleGeo returns the country breakdown.
func (s *Server) handleGeo(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	countries, err := s.events.Breakdown(r.Context(), q, storage.DimCountry)
	if err != nil {
		s.statsError(w, "geo", err)
		return
	}

	writeJSON(w, http.StatusOK, geoResponse{Countries: toCountJSON(countries)})
}

type techResponse struct {
	Devices  []countJSON `json:"devices"`
	Browsers []countJSON `json:"browsers"`
	OS       []countJSON `json:"os"`
}

// handleTech returns the device, browser, and OS breakdowns.
func (s *Server) handleTech(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	devices, err := s.events.Breakdown(r.Context(), q, storage.DimDevice)
	if err != nil {
		s.statsError(w, "tech: device", err)
		return
	}
	browsers, err := s.events.Breakdown(r.Context(), q, storage.DimBrowser)
	if err != nil {
		s.statsError(w, "tech: browser", err)
		return
	}
	oses, err := s.events.Breakdown(r.Context(), q, storage.DimOS)
	if err != nil {
		s.statsError(w, "tech: os", err)
		return
	}

	writeJSON(w, http.StatusOK, techResponse{
		Devices:  toCountJSON(devices),
		Browsers: toCountJSON(browsers),
		OS:       toCountJSON(oses),
	})
}

type eventsResponse struct {
	Events []eventJSON `json:"events"`

	// Props is present only when ?name= names an event.
	Props []propJSON `json:"props,omitempty"`
}

type eventJSON struct {
	Name     string `json:"name"`
	Count    int64  `json:"count"`
	Visitors int64  `json:"visitors"`
}

type propJSON struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// handleEvents returns custom event counts, and one event's property
// breakdown when ?name= names it.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	q, ok := s.buildQuery(w, r)
	if !ok {
		return
	}

	events, err := s.events.Events(r.Context(), q)
	if err != nil {
		s.statsError(w, "events", err)
		return
	}

	out := eventsResponse{Events: make([]eventJSON, 0, len(events))}
	for _, e := range events {
		out.Events = append(out.Events, eventJSON{Name: e.Name, Count: e.Count, Visitors: e.Visitors})
	}

	if name := strings.TrimSpace(r.URL.Query().Get("name")); name != "" {
		props, err := s.events.EventProps(r.Context(), q, name)
		if err != nil {
			s.statsError(w, "events: props", err)
			return
		}
		out.Props = make([]propJSON, 0, len(props))
		for _, p := range props {
			out.Props = append(out.Props, propJSON{Key: p.Key, Value: p.Value, Count: p.Count})
		}
	}

	writeJSON(w, http.StatusOK, out)
}

type realtimeResponse struct {
	Visitors int64 `json:"visitors"`

	// WindowSeconds says what "now" means, so the interface can label the
	// number without hardcoding a number the server owns.
	WindowSeconds int `json:"window_seconds"`
}

// handleRealtime returns visitors active right now.
func (s *Server) handleRealtime(w http.ResponseWriter, r *http.Request) {
	site, ok := s.resolveSite(w, r)
	if !ok {
		return
	}

	visitors, err := s.events.Realtime(r.Context(), site.ID)
	if err != nil {
		s.statsError(w, "realtime", err)
		return
	}

	writeJSON(w, http.StatusOK, realtimeResponse{
		Visitors:      visitors,
		WindowSeconds: int(storage.RealtimeWindow.Seconds()),
	})
}

func (s *Server) statsError(w http.ResponseWriter, what string, err error) {
	s.log.Error("stats: "+what, "err", err)
	writeError(w, http.StatusInternalServerError, "Couldn't load those stats. Try again.")
}

func toCountJSON(counts []storage.Count) []countJSON {
	out := make([]countJSON, 0, len(counts))
	for _, c := range counts {
		out = append(out, countJSON{Label: c.Label, Visitors: c.Visitors, Pageviews: c.Pageviews})
	}
	return out
}

func wantsCompare(r *http.Request) bool {
	return r.URL.Query().Get("compare") == "true"
}

// parseRange resolves ?from and ?to.
func parseRange(r *http.Request) (from, to time.Time, err error) {
	query := r.URL.Query()
	rawFrom := strings.TrimSpace(query.Get("from"))
	rawTo := strings.TrimSpace(query.Get("to"))

	now := time.Now().UTC()

	if rawFrom == "" && rawTo == "" {
		return now.Add(-defaultRange), now, nil
	}
	if rawFrom == "" || rawTo == "" {
		return time.Time{}, time.Time{}, errors.New("Give both from and to, or neither.")
	}

	from, err = parseTime(rawFrom, false)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("from must be a date (2026-07-17) or a timestamp.")
	}

	to, err = parseTime(rawTo, true)
	if err != nil {
		return time.Time{}, time.Time{}, errors.New("to must be a date (2026-07-17) or a timestamp.")
	}

	if !to.After(from) {
		return time.Time{}, time.Time{}, errors.New("to must be after from.")
	}
	return from, to, nil
}

// parseTime accepts a full timestamp or a bare date.
//
// A bare `to` date covers that whole day. "from=2026-07-01&to=2026-07-31"
// plainly means all of July, and a half-open range ending at July 31 00:00
// would silently drop the last day -- an off-by-one the caller would only
// notice by auditing the numbers.
func parseTime(raw string, endOfDay bool) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}

	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	if endOfDay {
		return t.UTC().AddDate(0, 0, 1), nil
	}
	return t.UTC(), nil
}

func parseLimit(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return 0, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be a number.")
	}
	if limit < 1 || limit > maxBreakdownLimit {
		return 0, errors.New("limit must be between 1 and 500.")
	}
	return limit, nil
}
