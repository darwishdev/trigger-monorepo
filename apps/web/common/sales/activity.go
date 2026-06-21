package sales

import "time"

// Activity models a scheduled follow-up task (Odoo: mail.activity).
type Activity struct {
	ID       string
	Summary  string
	Type     string
	Note     string
	Deadline time.Time
	State    string // overdue|today|planned
	UserID   string
	User     string
	LeadID   string
	Lead     string
}

// ActivityDraft is the payload needed to schedule a follow-up.
// ActivityType is provider-resolved (an Odoo activity xml id, or an Engaz
// equivalent); the adapter decides how to translate it.
type ActivityDraft struct {
	LeadID       string
	Summary      string
	Note         string
	Deadline     time.Time
	UserID       string // optional assignee
	ActivityType string // provider-resolved
}

// ActivityResult is returned by CompleteActivity. Odoo deletes the activity on
// completion and logs a chatter message; it maps that message id into AuditRef.
// Engaz will map its own task reference. The domain stays stable.
type ActivityResult struct {
	Completed Activity
	AuditRef  string
}

// ActivityFilter drives GET /api/activities. Pagination is keyset-based: pass
// the NextToken from a previous page to fetch the next one. Sort/Dir let the
// caller reorder results (provider-allowlisted); changing them invalidates an
// existing scroll token, restarting the scroll.
type ActivityFilter struct {
	ScrollToken string
	Sort        string // date_deadline | id (provider validates/defaults)
	Dir         string // asc (default) | desc
	PageSize    int
	State       string // overdue|today|planned
	Type        string // activity type name, e.g. Call / Email / Meeting
	UserID      string
	LeadID      string
}
