// Package adapter is the presentation boundary for the sales domain: it owns
// the request/view/result shapes that the api layer consumes and converts
// between them and the common/sales domain DTOs. It imports only common/sales,
// so the usecase can depend on it without cycles.
package adapter

import (
	"time"

	"trigger/apps/web/common/sales"
)

// ActivityListReq is the presentation-layer request for the activity list page.
type ActivityListReq struct {
	ScrollToken string
	Sort        string // date_deadline (default) | id
	Dir         string // asc (default) | desc
	PageSize    int
	State       string // overdue|today|planned (empty = all)
	Type        string // Call|Email|Meeting|To-Do (empty = all)
	UserID      string
	LeadID      string
}

// ActivityView is the per-row shape rendered on the activity list page.
type ActivityView struct {
	ID       string
	Summary  string
	Type     string
	Note     string
	Deadline time.Time
	State    string
	User     string
	Lead     string
}

// ActivityListResult is what the usecase returns to the api layer for rendering.
// Count is non-nil only on the first page (no scroll token).
type ActivityListResult struct {
	Items     []ActivityView
	Count     *int
	NextToken string
}

// ActivityListFilterDtoFromReq converts the presentation request into the
// domain filter passed to the CRM client.
func ActivityListFilterDtoFromReq(req ActivityListReq) sales.ActivityFilter {
	return sales.ActivityFilter{
		ScrollToken: req.ScrollToken,
		Sort:        req.Sort,
		Dir:         req.Dir,
		PageSize:    req.PageSize,
		State:       req.State,
		Type:        req.Type,
		UserID:      req.UserID,
		LeadID:      req.LeadID,
	}
}

// ActivityViewFromDto maps a single domain activity into its view shape.
func ActivityViewFromDto(a sales.Activity) ActivityView {
	return ActivityView{
		ID:       a.ID,
		Summary:  a.Summary,
		Type:     a.Type,
		Note:     a.Note,
		Deadline: a.Deadline,
		State:    a.State,
		User:     a.User,
		Lead:     a.Lead,
	}
}

// ActivityListViewFromDto maps a slice of domain activities into view shapes.
func ActivityListViewFromDto(items []sales.Activity) []ActivityView {
	views := make([]ActivityView, 0, len(items))
	for _, a := range items {
		views = append(views, ActivityViewFromDto(a))
	}
	return views
}
