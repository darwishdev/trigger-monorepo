package odooclient

import (
	"net/url"
	"strconv"
	"time"

	"trigger/apps/web/common/sales"
)

// Adapter: pure functions converting between sales domain DTOs and Odoo's
// request/response shapes. ALL conversion lives here - the client only knows
// endpoint paths and HTTP mechanics, nothing about field mapping.
//
// Naming convention: {Subject}{Target}From{Source}.
//   - Requests  (source Dto, target Odoo): LeadListFilterOdooFromDto, ...
//   - Responses (source Odoo, target Dto): LeadDtoFromOdoo, ...
//
// Odoo-isms are confined to this file and types.go; nothing in app/ or api/
// ever sees an odoo* type.

var dateLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02",
}

func parseTime(s *string) time.Time {
	if s == nil || *s == "" {
		return time.Time{}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, *s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func str(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func itoaPtr(i *int) string {
	if i == nil {
		return ""
	}
	return strconv.Itoa(*i)
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func money(amount float64, currency *string) sales.Money {
	return sales.Money{Amount: amount, Currency: str(currency)}
}

func setPaging(v url.Values, limit, offset int) {
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		v.Set("offset", strconv.Itoa(offset))
	}
}

// pageFromOdoo is the shared list-response mapper for the offset-based
// endpoints (leads, projects, units); mapFn converts one Odoo record into its
// domain DTO. These endpoints still use limit/offset, so NextToken is always
// empty here.
func pageFromOdoo[T, O any](p odooPage[O], mapFn func(O) T) sales.Page[T] {
	results := make([]T, 0, len(p.Results))
	for _, o := range p.Results {
		results = append(results, mapFn(o))
	}
	count := p.Count
	return sales.Page[T]{
		Results:   results,
		Count:     &count,
		NextToken: "",
	}
}

// ---- Request builders (Dto -> Odoo) ----------------------------------------

func LeadListFilterOdooFromDto(f sales.LeadFilter) url.Values {
	v := url.Values{}
	if f.Type != "" {
		v.Set("type", f.Type)
	}
	if f.Stage != "" {
		v.Set("stage", f.Stage)
	}
	if f.IncludeArchived {
		v.Set("include_archived", "1")
	}
	setPaging(v, f.Limit, f.Offset)
	return v
}

func ProjectListFilterOdooFromDto(f sales.ProjectFilter) url.Values {
	v := url.Values{}
	if f.Location != "" {
		v.Set("location", f.Location)
	}
	setPaging(v, f.Limit, f.Offset)
	return v
}

func UnitListFilterOdooFromDto(f sales.UnitFilter) url.Values {
	v := url.Values{}
	if f.ProjectID != "" {
		v.Set("project", f.ProjectID)
	}
	if f.State != "" {
		v.Set("state", f.State)
	}
	if f.MaxPrice > 0 {
		v.Set("max_price", strconv.FormatFloat(f.MaxPrice, 'f', -1, 64))
	}
	if f.MinBedrooms > 0 {
		v.Set("min_bedrooms", strconv.Itoa(f.MinBedrooms))
	}
	setPaging(v, f.Limit, f.Offset)
	return v
}

func ActivityListFilterOdooFromDto(f sales.ActivityFilter) url.Values {
	v := url.Values{}
	if f.ScrollToken != "" {
		v.Set("scroll_token", f.ScrollToken)
	}
	if f.Sort != "" {
		v.Set("sort", f.Sort)
	}
	if f.Dir != "" {
		v.Set("dir", f.Dir)
	}
	if f.PageSize > 0 {
		v.Set("page_size", strconv.Itoa(f.PageSize))
	}
	if f.State != "" {
		v.Set("state", f.State)
	}
	if f.Type != "" {
		v.Set("activity_type", f.Type)
	}
	if f.UserID != "" {
		v.Set("user", f.UserID)
	}
	if f.LeadID != "" {
		v.Set("lead", f.LeadID)
	}
	return v
}

// ActivityScheduleOdooFromDto builds the POST /api/activities body. The
// activity type defaults to Odoo's generic "todo" xml id when unset, the
// deadline is formatted as Odoo's expected YYYY-MM-DD, and the assignee is
// parsed from its string form.
func ActivityScheduleOdooFromDto(d sales.ActivityDraft) scheduleActivityRequest {
	body := scheduleActivityRequest{
		LeadID:       d.LeadID,
		Summary:      d.Summary,
		Note:         d.Note,
		ActivityType: d.ActivityType,
	}
	if body.ActivityType == "" {
		body.ActivityType = "mail.mail_activity_data_todo"
	}
	if !d.Deadline.IsZero() {
		body.DateDeadline = d.Deadline.Format("2006-01-02")
	}
	if d.UserID != "" {
		body.UserID = atoi(d.UserID)
	}
	return body
}

func ActivityCompleteParamsFromDto(feedback string) url.Values {
	v := url.Values{}
	if feedback != "" {
		v.Set("feedback", feedback)
	}
	return v
}

// ---- Response mappers (Odoo -> Dto) ----------------------------------------

func ProjectRefDtoFromOdoo(o odooProjectRef) sales.ProjectRef {
	return sales.ProjectRef{
		ID:       strconv.Itoa(o.ID),
		Name:     o.Name,
		Location: o.Location,
	}
}

func LeadDtoFromOdoo(o odooLead) sales.Lead {
	projects := make([]sales.ProjectRef, 0, len(o.SuggestedProjects))
	for _, p := range o.SuggestedProjects {
		projects = append(projects, ProjectRefDtoFromOdoo(p))
	}
	tags := o.Tags
	if tags == nil {
		tags = []string{}
	}
	return sales.Lead{
		ID:                strconv.Itoa(o.ID),
		Name:              o.Name,
		Type:              o.Type,
		ContactName:       str(o.ContactName),
		Email:             str(o.Email),
		Phone:             str(o.Phone),
		Stage:             str(o.Stage),
		Budget:            money(o.Budget, o.Currency),
		Probability:       o.Probability,
		Priority:          o.Priority,
		Location:          str(o.Location),
		Salesperson:       str(o.Salesperson),
		SalespersonID:     itoaPtr(o.SalespersonID),
		Tags:              tags,
		Active:            o.Active,
		SuggestedProjects: projects,
	}
}

func LeadListDtoFromOdoo(p odooPage[odooLead]) sales.Page[sales.Lead] {
	return pageFromOdoo(p, LeadDtoFromOdoo)
}

func LeadDetailDtoFromOdoo(o odooLeadDetail) sales.LeadDetail {
	activities := make([]sales.Activity, 0, len(o.OpenActivities))
	for _, a := range o.OpenActivities {
		activities = append(activities, ActivityDtoFromOdoo(a))
	}
	return sales.LeadDetail{
		Lead:           LeadDtoFromOdoo(o.odooLead),
		Partner:        str(o.Partner),
		SalesTeam:      str(o.SalesTeam),
		Deadline:       parseTime(o.DateDeadline),
		OpenActivities: activities,
	}
}

func ProjectDtoFromOdoo(o odooProject) sales.Project {
	return sales.Project{
		ID:             strconv.Itoa(o.ID),
		Name:           o.Name,
		Location:       str(o.Location),
		Developer:      str(o.Developer),
		DeliveryDate:   parseTime(o.DeliveryDate),
		UnitCount:      o.UnitCount,
		AvailableUnits: o.AvailableUnits,
		PriceFrom:      money(o.PriceFrom, o.Currency),
	}
}

func ProjectListDtoFromOdoo(p odooPage[odooProject]) sales.Page[sales.Project] {
	return pageFromOdoo(p, ProjectDtoFromOdoo)
}

func ProjectDetailDtoFromOdoo(o odooProjectDetail) sales.ProjectDetail {
	units := make([]sales.Unit, 0, len(o.Units))
	for _, u := range o.Units {
		units = append(units, UnitDtoFromOdoo(u))
	}
	return sales.ProjectDetail{
		Project: ProjectDtoFromOdoo(o.odooProject),
		Units:   units,
	}
}

func UnitDtoFromOdoo(o odooUnit) sales.Unit {
	return sales.Unit{
		ID:        strconv.Itoa(o.ID),
		Code:      str(o.Code),
		Name:      o.Name,
		ProjectID: strconv.Itoa(o.ProjectID),
		Project:   str(o.Project),
		Type:      str(o.Type),
		AreaSqm:   o.AreaSqm,
		Bedrooms:  o.Bedrooms,
		Bathrooms: o.Bathrooms,
		Floor:     str(o.Floor),
		Price:     money(o.Price, o.Currency),
		State:     str(o.State),
	}
}

func UnitListDtoFromOdoo(p odooPage[odooUnit]) sales.Page[sales.Unit] {
	return pageFromOdoo(p, UnitDtoFromOdoo)
}

func ActivityDtoFromOdoo(o odooActivity) sales.Activity {
	return sales.Activity{
		ID:       strconv.Itoa(o.ID),
		Summary:  str(o.Summary),
		Type:     str(o.Type),
		Note:     str(o.Note),
		Deadline: parseTime(o.DateDeadline),
		State:    o.State,
		UserID:   itoaPtr(o.UserID),
		User:     str(o.User),
		LeadID:   itoaPtr(o.LeadID),
		Lead:     str(o.Lead),
	}
}

func ActivityListDtoFromOdoo(p odooActivityListResponse) sales.Page[sales.Activity] {
	results := make([]sales.Activity, 0, len(p.Results))
	for _, o := range p.Results {
		results = append(results, ActivityDtoFromOdoo(o))
	}
	return sales.Page[sales.Activity]{
		Results:   results,
		Count:     p.Count,
		NextToken: p.NextScrollToken,
	}
}

// ActivityResultDtoFromOdoo maps the done-response. Odoo deletes the activity
// on completion and logs a chatter message; that message id becomes AuditRef.
func ActivityResultDtoFromOdoo(o activityDoneResponse) sales.ActivityResult {
	result := sales.ActivityResult{Completed: ActivityDtoFromOdoo(o.CompletedActivity)}
	if o.MessageID != nil {
		result.AuditRef = strconv.Itoa(*o.MessageID)
	}
	return result
}
