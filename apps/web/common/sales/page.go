package sales

// Money groups a monetary amount with its currency code.
// Odoo models expected_revenue / list_price / price_from as a float64 plus a
// separate currency name; we preserve that shape behind a single clean type.
type Money struct {
	Amount   float64
	Currency string
}

// Page is the generic list envelope every collection endpoint returns.
//
// Pagination is keyset-based: callers echo NextToken from a previous response
// to fetch the next page; an empty NextToken means there are no more results.
// Count holds the total matching the filter and is only populated on the first
// page (when the scroll token is empty) - it is nil on subsequent pages so the
// provider can skip re-running a COUNT on every scroll.
type Page[T any] struct {
	Results   []T
	Count     *int
	NextToken string
}
