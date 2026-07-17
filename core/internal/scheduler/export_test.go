package scheduler

import (
	"time"

	"github.com/zenith/core/internal/email"
)

// SetNow overrides the clock so a test can name the month it reports on
// instead of waiting for one. Compiled only under test.
func SetNow(r *Reporter, now func() time.Time) { r.now = now }

// SetSender substitutes the mail transport, so the suite can assert on what
// would have been sent without sending it. Compiled only under test.
func SetSender(r *Reporter, s email.Sender) { r.sender = s }
