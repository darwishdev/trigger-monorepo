{
    "name": "Trigger CRM REST API",
    "version": "18.0.1.0.0",
    "summary": "Lightweight REST endpoints to consume Trigger CRM data (leads, opportunities, ...).",
    "description": """
Trigger CRM REST API
====================
Exposes simple REST endpoints (JSON over HTTP) on top of the Odoo CRM so
external clients can consume Trigger's leads/opportunities using Odoo
Bearer API keys.

Endpoints:
  GET /api/leads   List leads & opportunities (filter, paginate).
""",
    "author": "Trigger",
    "website": "https://www.trigger-realestate.com",
    "category": "Sales/CRM",
    "license": "LGPL-3",
    "depends": ["crm", "product", "trigger_estate"],
    "data": [],
    "installable": True,
    "application": False,
    "auto_install": False,
}
