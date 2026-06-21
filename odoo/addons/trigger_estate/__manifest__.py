{
    "name": "Trigger Estate (Projects & Units)",
    "version": "18.0.1.0.0",
    "summary": "Real-estate projects (product categories) and units (products) for Trigger CRM.",
    "description": """
Trigger Estate
==============
Models the company's sellable inventory by reusing Odoo's product backbone:

  * Project  -> product.category  (extended: is_project, location, developer,
                delivery date, computed price_from / unit counts)
  * Unit     -> product.template  (extended: unit_state, type, area, bedrooms,
                bathrooms, floor) linked to its project via categ_id

It also adds `suggested_category_ids` (suggested projects) on crm.lead so reps
can match leads to developments.
""",
    "author": "Trigger",
    "website": "https://www.trigger-realestate.com",
    "category": "Sales/CRM",
    "license": "LGPL-3",
    "depends": ["crm", "product"],
    "data": [],
    "demo": [
        "demo/estate_projects_demo.xml",
        "demo/estate_units_demo.xml",
    ],
    "installable": True,
    "application": False,
    "auto_install": False,
}
