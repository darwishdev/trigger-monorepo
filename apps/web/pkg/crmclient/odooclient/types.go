package odooclient

// Odoo JSON structs mirror the output of trigger_crm_api/controllers/main.py
// _serialize_* helpers exactly. Nullable fields use pointers so JSON null
// deserializes cleanly; the adapter unwraps them when building domain DTOs.
// All Odoo-isms live in this file and adapter.go - they never escape.

type odooPage[T any] struct {
	Count   int `json:"count"`
	Limit   int `json:"limit"`
	Offset  int `json:"offset"`
	Results []T `json:"results"`
}

type odooProjectRef struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Location string `json:"location"`
}

type odooLead struct {
	ID                int              `json:"id"`
	Name              string           `json:"name"`
	Type              string           `json:"type"`
	ContactName       *string          `json:"contact_name"`
	Email             *string          `json:"email"`
	Phone             *string          `json:"phone"`
	Stage             *string          `json:"stage"`
	Budget            float64          `json:"budget"`
	Currency          *string          `json:"currency"`
	Probability       float64          `json:"probability"`
	Priority          string           `json:"priority"`
	Location          *string          `json:"location"`
	Salesperson       *string          `json:"salesperson"`
	SalespersonID     *int             `json:"salesperson_id"`
	Tags              []string         `json:"tags"`
	Active            bool             `json:"active"`
	SuggestedProjects []odooProjectRef `json:"suggested_projects"`
}

// odooLeadDetail mirrors odooLead with the extra fields only present on the
// detail endpoint. We embed by value via flattening in adapter, so JSON tags
// must not collide; we duplicate the base fields here for direct decoding.
type odooLeadDetail struct {
	odooLead
	Partner        *string        `json:"partner"`
	SalesTeam      *string        `json:"sales_team"`
	DateDeadline   *string        `json:"date_deadline"`
	OpenActivities []odooActivity `json:"open_activities"`
}

type odooProject struct {
	ID             int     `json:"id"`
	Name           string  `json:"name"`
	Location       *string `json:"location"`
	Developer      *string `json:"developer"`
	DeliveryDate   *string `json:"delivery_date"`
	UnitCount      int     `json:"unit_count"`
	AvailableUnits int     `json:"available_units"`
	PriceFrom      float64 `json:"price_from"`
	Currency       *string `json:"currency"`
}

type odooProjectDetail struct {
	odooProject
	Units []odooUnit `json:"units"`
}

type odooUnit struct {
	ID        int     `json:"id"`
	Code      *string `json:"code"`
	Name      string  `json:"name"`
	Project   *string `json:"project"`
	ProjectID int     `json:"project_id"`
	Type      *string `json:"type"`
	AreaSqm   float64 `json:"area_sqm"`
	Bedrooms  int     `json:"bedrooms"`
	Bathrooms int     `json:"bathrooms"`
	Floor     *string `json:"floor"`
	Price     float64 `json:"price"`
	Currency  *string `json:"currency"`
	State     *string `json:"state"`
}

type odooActivity struct {
	ID           int     `json:"id"`
	Summary      *string `json:"summary"`
	Type         *string `json:"type"`
	Note         *string `json:"note"`
	DateDeadline *string `json:"date_deadline"`
	State        string  `json:"state"`
	UserID       *int    `json:"user_id"`
	User         *string `json:"user"`
	LeadID       *int    `json:"lead_id"`
	Lead         *string `json:"lead"`
	ResModel     string  `json:"res_model"`
}

// odooActivityListResponse is the keyset-paginated envelope returned by
// GET /api/activities. Count is present only on the first page (no scroll
// token); next_scroll_token is present when there are more results.
type odooActivityListResponse struct {
	Count           *int           `json:"count,omitempty"`
	Results         []odooActivity `json:"results"`
	NextScrollToken string         `json:"next_scroll_token,omitempty"`
}

// activityDoneResponse is the body returned by POST /api/activities/{id}/done.
type activityDoneResponse struct {
	OK                bool         `json:"ok"`
	CompletedActivity odooActivity `json:"completed_activity"`
	MessageID         *int         `json:"message_id"`
}

// scheduleActivityRequest is the JSON body for POST /api/activities.
type scheduleActivityRequest struct {
	LeadID       string `json:"lead"`
	Summary      string `json:"summary,omitempty"`
	Note         string `json:"note,omitempty"`
	DateDeadline string `json:"date_deadline,omitempty"`
	UserID       int    `json:"user_id,omitempty"`
	ActivityType string `json:"activity_type,omitempty"`
}
