package sales

// Unit models a sellable real-estate unit (Odoo: product.template where
// is_property = True), linked to its project via project_id.
type Unit struct {
	ID        string
	Code      string
	Name      string
	ProjectID string
	Project   string
	Type      string // apartment|villa|townhouse|office|retail
	AreaSqm   float64
	Bedrooms  int
	Bathrooms int
	Floor     string
	Price     Money  // Odoo: list_price
	State     string // available|reserved|sold
}

// UnitFilter mirrors the query parameters of GET /api/units.
type UnitFilter struct {
	ProjectID   string
	State       string // available|reserved|sold
	MaxPrice    float64
	MinBedrooms int
	Limit       int
	Offset      int
}
