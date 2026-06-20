package usecase

import (
	"context"

	"trigger/apps/web/app/sales/adapter"
)

// ActivityList drives the follow-up activity list page. It resolves the
// tenant's cached CRM client, asks for activities matching the request's
// filter, and maps them to the presentation result.
func (u *Usecase) ActivityList(ctx context.Context, req adapter.ActivityListReq) (adapter.ActivityListResult, error) {
	crm, err := u.reg.Build(defaultTenant, u.cfg)
	if err != nil {
		return adapter.ActivityListResult{}, err
	}

	page, err := crm.ActivityList(ctx, adapter.ActivityListFilterDtoFromReq(req))
	if err != nil {
		return adapter.ActivityListResult{}, err
	}

	return adapter.ActivityListResult{
		Items:     adapter.ActivityListViewFromDto(page.Results),
		Count:     page.Count,
		NextToken: page.NextToken,
	}, nil
}
