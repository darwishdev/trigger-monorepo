{
    "name": "Trigger Real Estate CRM Demo Data",
    "version": "18.0.1.0.0",
    "summary": "Demo data for Trigger Real Estate CRM in Egypt (leads, opportunities, contacts).",
    "description": """
Trigger Real Estate CRM Demo Data (Egypt)
=========================================
Populates the CRM with realistic demo records for Trigger, a real estate
company operating in Egypt: contacts (buyers, sellers, landlords, tenants)
based in Cairo, Giza and Alexandria, leads and opportunities covering
sales, rentals and inquiries across New Cairo, Sheikh Zayed, the New
Administrative Capital and the North Coast (all priced in EGP), plus
related activities (calls, meetings, emails) and a lost-reason set.
""",
    "author": "Trigger",
    "website": "https://www.trigger.example.com",
    "category": "Sales/Crm",
    "license": "LGPL-3",
    "depends": [
        "crm",
        "contacts",
        "mail",
        "trigger_estate",
    ],
    "data": [],
    "demo": [
        "demo/res_partner_demo.xml",
        "demo/crm_tag_demo.xml",
        "demo/crm_lost_reason_demo.xml",
        "demo/crm_lead_demo.xml",
        "demo/mail_activity_demo.xml",
    ],
    "installable": True,
    "application": False,
    "auto_install": False,
}
