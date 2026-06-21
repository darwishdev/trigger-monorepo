from odoo import fields, models


class CrmLead(models.Model):
    _inherit = "crm.lead"

    suggested_category_ids = fields.Many2many(
        "product.category",
        string="Suggested Projects",
        domain="[('is_project', '=', True)]",
        help="Developments suggested to this lead by the sales rep.",
    )
