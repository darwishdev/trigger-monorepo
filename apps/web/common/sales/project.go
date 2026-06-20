package sales

import "time"

// Project models a real-estate development (Odoo: product.category where
// is_project = True). Project is the aggregate root; its Units are children.
type Project struct {
	ID             string
	Name           string
	Location       string
	Developer      string
	DeliveryDate   time.Time
	UnitCount      int
	AvailableUnits int
	PriceFrom      Money
}

// ProjectDetail is the single-resource view returned by GET /api/projects/{id};
// it nests the project's units.
type ProjectDetail struct {
	Project
	Units []Unit
}

// ProjectFilter mirrors the query parameters of GET /api/projects.
type ProjectFilter struct {
	Location string
	Limit    int
	Offset   int
}
