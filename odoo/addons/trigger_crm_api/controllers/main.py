import json

from odoo import fields, http
from odoo.http import request


class TriggerCrmApi(http.Controller):
    """Read-only REST endpoints for consuming Trigger CRM data, plus a
    write endpoint to complete (mark done) follow-up activities."""

    # ------------------------------------------------------------------ #
    #  helpers
    # ------------------------------------------------------------------ #
    @staticmethod
    def _page(params):
        """Parse limit/offset, raising ValueError on bad input."""
        limit = min(max(int(params.get("limit", 80)), 1), 500)
        offset = max(int(params.get("offset", 0)), 0)
        return limit, offset

    # ------------------------------------------------------------------ #
    #  Leads
    # ------------------------------------------------------------------ #
    @http.route("/api/leads", type="http", auth="bearer", methods=["GET"], csrf=False)
    def list_leads(self, **params):
        Lead = request.env["crm.lead"]
        try:
            limit, offset = self._page(params)
        except (TypeError, ValueError):
            return request.make_json_response({"error": "limit/offset must be integers"}, status=400)

        order = params.get("order") or "expected_revenue desc"
        domain = []
        lead_type = params.get("type")
        if lead_type:
            if lead_type not in ("lead", "opportunity"):
                return request.make_json_response({"error": "type must be 'lead' or 'opportunity'"}, status=400)
            domain.append(("type", "=", lead_type))
        stage = params.get("stage")
        if stage:
            domain.append(("stage_id.name", "=", stage))
        if params.get("include_archived") in ("1", "true", "True", "yes"):
            Lead = Lead.with_context(active_test=False)

        total = Lead.search_count(domain)
        records = Lead.search(domain, limit=limit, offset=offset, order=order)
        return request.make_json_response({
            "count": total, "limit": limit, "offset": offset,
            "results": [self._serialize_lead(r) for r in records],
        })

    @http.route("/api/leads/<int:lead_id>", type="http", auth="bearer", methods=["GET"], csrf=False)
    def get_lead(self, lead_id, **params):
        lead = request.env["crm.lead"].with_context(active_test=False).browse(lead_id)
        if not lead.exists():
            return request.make_json_response({"error": "lead not found"}, status=404)
        return request.make_json_response(self._serialize_lead(lead, detail=True))

    def _serialize_lead(self, lead, detail=False):
        data = {
            "id": lead.id,
            "name": lead.name,
            "type": lead.type,
            "contact_name": lead.contact_name or None,
            "email": lead.email_from or None,
            "phone": lead.phone or None,
            "stage": lead.stage_id.name or None,
            "budget": lead.expected_revenue,
            "currency": lead.company_currency.name or None,
            "probability": lead.probability,
            "priority": lead.priority,
            "location": lead.city or (lead.partner_id.city or None),
            "salesperson": lead.user_id.name or None,
            "salesperson_id": lead.user_id.id or None,
            "tags": lead.tag_ids.mapped("name"),
            "active": lead.active,
            "suggested_projects": [
                {"id": c.id, "name": c.name, "location": c.location}
                for c in lead.suggested_category_ids
            ],
        }
        if detail:
            data["partner"] = lead.partner_id.display_name or None
            data["sales_team"] = lead.team_id.name or None
            data["date_deadline"] = lead.date_deadline.isoformat() if lead.date_deadline else None
            data["open_activities"] = [
                self._serialize_activity(a) for a in lead.activity_ids
            ]
        return data

    # ------------------------------------------------------------------ #
    #  Projects (product.category, is_project)
    # ------------------------------------------------------------------ #
    @http.route("/api/projects", type="http", auth="bearer", methods=["GET"], csrf=False)
    def list_projects(self, **params):
        Categ = request.env["product.category"]
        try:
            limit, offset = self._page(params)
        except (TypeError, ValueError):
            return request.make_json_response({"error": "limit/offset must be integers"}, status=400)

        domain = [("is_project", "=", True)]
        location = params.get("location")
        if location:
            domain.append(("location", "ilike", location))

        total = Categ.search_count(domain)
        records = Categ.search(domain, limit=limit, offset=offset, order="name")
        return request.make_json_response({
            "count": total, "limit": limit, "offset": offset,
            "results": [self._serialize_project(r) for r in records],
        })

    @http.route("/api/projects/<int:project_id>", type="http", auth="bearer", methods=["GET"], csrf=False)
    def get_project(self, project_id, **params):
        project = request.env["product.category"].browse(project_id)
        if not project.exists() or not project.is_project:
            return request.make_json_response({"error": "project not found"}, status=404)
        data = self._serialize_project(project)
        data["units"] = [self._serialize_unit(u) for u in project.project_unit_ids]
        return request.make_json_response(data)

    def _serialize_project(self, categ):
        return {
            "id": categ.id,
            "name": categ.name,
            "location": categ.location or None,
            "developer": categ.developer_id.name or None,
            "delivery_date": categ.delivery_date.isoformat() if categ.delivery_date else None,
            "unit_count": categ.unit_count,
            "available_units": categ.available_unit_count,
            "price_from": categ.price_from,
            "currency": categ.currency_id.name or None,
        }

    # ------------------------------------------------------------------ #
    #  Units (product.template, is_property)
    # ------------------------------------------------------------------ #
    @http.route("/api/units", type="http", auth="bearer", methods=["GET"], csrf=False)
    def list_units(self, **params):
        Tmpl = request.env["product.template"]
        try:
            limit, offset = self._page(params)
        except (TypeError, ValueError):
            return request.make_json_response({"error": "limit/offset must be integers"}, status=400)

        domain = [("is_property", "=", True)]
        if params.get("project"):
            try:
                domain.append(("categ_id", "=", int(params["project"])))
            except ValueError:
                domain.append(("categ_id.name", "=", params["project"]))
        state = params.get("state")
        if state:
            if state not in ("available", "reserved", "sold"):
                return request.make_json_response({"error": "state must be available/reserved/sold"}, status=400)
            domain.append(("unit_state", "=", state))
        if params.get("max_price"):
            try:
                domain.append(("list_price", "<=", float(params["max_price"])))
            except ValueError:
                return request.make_json_response({"error": "max_price must be a number"}, status=400)
        if params.get("min_bedrooms"):
            try:
                domain.append(("bedrooms", ">=", int(params["min_bedrooms"])))
            except ValueError:
                return request.make_json_response({"error": "min_bedrooms must be an integer"}, status=400)

        total = Tmpl.search_count(domain)
        records = Tmpl.search(domain, limit=limit, offset=offset, order="list_price")
        return request.make_json_response({
            "count": total, "limit": limit, "offset": offset,
            "results": [self._serialize_unit(r) for r in records],
        })

    def _serialize_unit(self, tmpl):
        return {
            "id": tmpl.id,
            "code": tmpl.unit_code or None,
            "name": tmpl.name,
            "project": tmpl.categ_id.name or None,
            "project_id": tmpl.categ_id.id,
            "type": tmpl.property_type or None,
            "area_sqm": tmpl.area_sqm,
            "bedrooms": tmpl.bedrooms,
            "bathrooms": tmpl.bathrooms,
            "floor": tmpl.floor or None,
            "price": tmpl.list_price,
            "currency": tmpl.currency_id.name or None,
            "state": tmpl.unit_state or None,
        }

    # ------------------------------------------------------------------ #
    #  Activities (mail.activity) - the pending task queue
    # ------------------------------------------------------------------ #
    @http.route("/api/activities", type="http", auth="bearer", methods=["GET"], csrf=False)
    def list_activities(self, **params):
        Activity = request.env["mail.activity"]
        try:
            limit, offset = self._page(params)
        except (TypeError, ValueError):
            return request.make_json_response({"error": "limit/offset must be integers"}, status=400)

        domain = [("res_model", "=", params.get("model") or "crm.lead")]
        if params.get("user"):
            try:
                domain.append(("user_id", "=", int(params["user"])))
            except ValueError:
                return request.make_json_response({"error": "user must be an integer id"}, status=400)
        if params.get("lead"):
            try:
                domain.append(("res_id", "=", int(params["lead"])))
            except ValueError:
                return request.make_json_response({"error": "lead must be an integer id"}, status=400)
        state = params.get("state")
        if state:
            if state not in ("overdue", "today", "planned"):
                return request.make_json_response({"error": "state must be overdue/today/planned"}, status=400)
            # mail.activity.state is a non-stored computed field and cannot be
            # searched directly; filter on date_deadline relative to today.
            today = fields.Date.context_today(request.env.user)
            if state == "overdue":
                domain.append(("date_deadline", "<", today))
            elif state == "today":
                domain.append(("date_deadline", "=", today))
            else:  # planned
                domain.append(("date_deadline", ">", today))

        total = Activity.search_count(domain)
        records = Activity.search(domain, limit=limit, offset=offset, order="date_deadline asc")
        return request.make_json_response({
            "count": total, "limit": limit, "offset": offset,
            "results": [self._serialize_activity(r) for r in records],
        })

    @http.route("/api/activities", type="http", auth="bearer", methods=["POST"], csrf=False)
    def create_activity(self, **params):
        """Schedule a follow-up activity on a lead.

        Accepts a JSON body or form params:
          lead / res_id     (required) crm.lead id
          summary           short title
          note              longer description
          date_deadline     'YYYY-MM-DD' (defaults to today)
          user_id           assignee (defaults to current user)
          activity_type_id  int, OR
          activity_type     xmlid (default 'mail.mail_activity_data_todo')
        """
        data = dict(params)
        raw = request.httprequest.get_data(as_text=True)
        if raw:
            try:
                body = json.loads(raw)
                if isinstance(body, dict):
                    data.update(body)
            except (ValueError, TypeError):
                pass

        lead_id = data.get("lead") or data.get("res_id")
        if not lead_id:
            return request.make_json_response({"error": "lead (res_id) is required"}, status=400)
        try:
            lead_id = int(lead_id)
        except (TypeError, ValueError):
            return request.make_json_response({"error": "lead must be an integer id"}, status=400)

        lead = request.env["crm.lead"].browse(lead_id)
        if not lead.exists():
            return request.make_json_response({"error": "lead not found"}, status=404)

        act_values = {}
        if data.get("user_id"):
            try:
                act_values["user_id"] = int(data["user_id"])
            except (TypeError, ValueError):
                return request.make_json_response({"error": "user_id must be an integer"}, status=400)

        act_type_xmlid = data.get("activity_type") or "mail.mail_activity_data_todo"
        if data.get("activity_type_id"):
            try:
                act_values["activity_type_id"] = int(data["activity_type_id"])
                act_type_xmlid = ""  # explicit id wins
            except (TypeError, ValueError):
                return request.make_json_response({"error": "activity_type_id must be an integer"}, status=400)

        try:
            activity = lead.activity_schedule(
                act_type_xmlid,
                date_deadline=data.get("date_deadline") or None,
                summary=data.get("summary") or "",
                note=data.get("note") or "",
                **act_values,
            )
        except Exception as exc:  # noqa: BLE001 - surface a clean API error
            return request.make_json_response({"error": str(exc)}, status=400)

        return request.make_json_response(self._serialize_activity(activity), status=201)

    @http.route("/api/activities/<int:activity_id>/done", type="http", auth="bearer", methods=["POST"], csrf=False)
    def complete_activity(self, activity_id, **params):
        activity = request.env["mail.activity"].browse(activity_id)
        if not activity.exists():
            return request.make_json_response({"error": "activity not found"}, status=404)

        # Snapshot before completion - Odoo DELETES the activity on done and
        # logs a chatter message on the record instead.
        snapshot = self._serialize_activity(activity)
        res_model, res_id = activity.res_model, activity.res_id
        activity.action_feedback(feedback=params.get("feedback") or "")

        message = request.env["mail.message"].search(
            [("model", "=", res_model), ("res_id", "=", res_id)],
            order="id desc", limit=1,
        )
        return request.make_json_response({
            "ok": True,
            "completed_activity": snapshot,
            "message_id": message.id or None,  # chatter trace in Odoo
        })

    def _serialize_activity(self, act):
        return {
            "id": act.id,
            "summary": act.summary or (act.activity_type_id.name or None),
            "type": act.activity_type_id.name or None,
            "note": act.note or None,
            "date_deadline": act.date_deadline.isoformat() if act.date_deadline else None,
            "state": act.state,  # overdue | today | planned
            "user_id": act.user_id.id or None,
            "user": act.user_id.name or None,
            "lead_id": act.res_id if act.res_model == "crm.lead" else None,
            "lead": act.res_name or None,
            "res_model": act.res_model,
        }
