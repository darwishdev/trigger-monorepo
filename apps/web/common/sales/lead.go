package sales

import "time"

// ProjectRef is the lightweight project reference embedded on a Lead
// (Odoo: suggested_projects).
type ProjectRef struct {
	ID       string
	Name     string
	Location string
}

// Lead models a CRM Lead / Opportunity (Odoo: crm.lead).
type Lead struct {
	ID                string
	Name              string
	Type              string // "lead" | "opportunity"
	ContactName       string
	Email             string
	Phone             string
	Stage             string
	Budget            Money // Odoo: expected_revenue
	Probability       float64
	Priority          string
	Location          string
	Salesperson       string
	SalespersonID     string
	Tags              []string
	Active            bool
	SuggestedProjects []ProjectRef
}

// LeadDetail is the single-resource view; adds fields only present on
// GET /api/leads/{id}.
type LeadDetail struct {
	Lead
	Partner        string
	SalesTeam      string
	Deadline       time.Time
	OpenActivities []Activity
}

// LeadFilter mirrors the query parameters of GET /api/leads.
type LeadFilter struct {
	Type            string
	Stage           string
	IncludeArchived bool
	Limit           int
	Offset          int
}
