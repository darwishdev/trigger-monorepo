// Package odooclient implements sales.CRM against a Trigger CRM REST API
// running on Odoo 18 (the trigger_crm_api addon). The client owns only the
// endpoint paths; every field-level conversion lives in adapter.go, and the
// HTTP mechanics live in the reusable common/httpclient.Caller.
package odooclient

import (
	"context"
	"fmt"
	"net/url"

	"trigger/apps/web/common/httpclient"
	"trigger/apps/web/common/sales"
)

// Client implements sales.CRM against a Trigger Odoo instance. Interface
// satisfaction is enforced at the NewClient return statement (which returns
// sales.CRM), so no separate compile-time assertion is needed.
type Client struct {
	caller *httpclient.Caller
}

// NewClient is the builder registered into the crmclient registry. It validates
// the config up front so malformed tenant configs fail fast, builds the
// transport via common/httpclient, and injects the sales error mapper so
// non-success responses become typed sales errors.
func NewClient(cfg sales.CRMConfig) (sales.CRM, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("odooclient: empty base URL")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("odooclient: empty API key")
	}
	restyClient := httpclient.New(cfg.BaseURL, httpclient.WithBearerToken(cfg.APIKey))
	return &Client{caller: httpclient.NewCaller(restyClient, sales.MapError)}, nil
}

// ---- Leads -----------------------------------------------------------------

func (c *Client) LeadList(ctx context.Context, f sales.LeadFilter) (sales.Page[sales.Lead], error) {
	var page odooPage[odooLead]
	if err := c.caller.Get(ctx, "/api/leads", LeadListFilterOdooFromDto(f), &page); err != nil {
		return sales.Page[sales.Lead]{}, err
	}
	return LeadListDtoFromOdoo(page), nil
}

func (c *Client) LeadFind(ctx context.Context, id string) (sales.LeadDetail, error) {
	var detail odooLeadDetail
	if err := c.caller.Get(ctx, "/api/leads/"+url.PathEscape(id), url.Values{}, &detail); err != nil {
		return sales.LeadDetail{}, err
	}
	return LeadDetailDtoFromOdoo(detail), nil
}

// ---- Projects --------------------------------------------------------------

func (c *Client) ProjectList(ctx context.Context, f sales.ProjectFilter) (sales.Page[sales.Project], error) {
	var page odooPage[odooProject]
	if err := c.caller.Get(ctx, "/api/projects", ProjectListFilterOdooFromDto(f), &page); err != nil {
		return sales.Page[sales.Project]{}, err
	}
	return ProjectListDtoFromOdoo(page), nil
}

func (c *Client) ProjectFind(ctx context.Context, id string) (sales.ProjectDetail, error) {
	var detail odooProjectDetail
	if err := c.caller.Get(ctx, "/api/projects/"+url.PathEscape(id), url.Values{}, &detail); err != nil {
		return sales.ProjectDetail{}, err
	}
	return ProjectDetailDtoFromOdoo(detail), nil
}

// ---- Units -----------------------------------------------------------------

func (c *Client) UnitList(ctx context.Context, f sales.UnitFilter) (sales.Page[sales.Unit], error) {
	var page odooPage[odooUnit]
	if err := c.caller.Get(ctx, "/api/units", UnitListFilterOdooFromDto(f), &page); err != nil {
		return sales.Page[sales.Unit]{}, err
	}
	return UnitListDtoFromOdoo(page), nil
}

// ---- Activities ------------------------------------------------------------

func (c *Client) ActivityList(ctx context.Context, f sales.ActivityFilter) (sales.Page[sales.Activity], error) {
	var resp odooActivityListResponse
	if err := c.caller.Get(ctx, "/api/activities", ActivityListFilterOdooFromDto(f), &resp); err != nil {
		return sales.Page[sales.Activity]{}, err
	}
	return ActivityListDtoFromOdoo(resp), nil
}

func (c *Client) ActivitySchedule(ctx context.Context, draft sales.ActivityDraft) (sales.Activity, error) {
	var created odooActivity
	body := ActivityScheduleOdooFromDto(draft)
	if err := c.caller.Post(ctx, "/api/activities", url.Values{}, body, &created); err != nil {
		return sales.Activity{}, err
	}
	return ActivityDtoFromOdoo(created), nil
}

func (c *Client) ActivityComplete(ctx context.Context, id, feedback string) (sales.ActivityResult, error) {
	var done activityDoneResponse
	if err := c.caller.Post(ctx, "/api/activities/"+url.PathEscape(id)+"/done",
		ActivityCompleteParamsFromDto(feedback), nil, &done); err != nil {
		return sales.ActivityResult{}, err
	}
	return ActivityResultDtoFromOdoo(done), nil
}
