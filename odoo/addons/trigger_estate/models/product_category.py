from odoo import api, fields, models


class ProductCategory(models.Model):
    """A product category doubles as a real-estate Project.

    Reusing product.category gives us the project -> units link for free
    (a unit's categ_id is its project) without a dedicated table.
    """

    _inherit = "product.category"

    is_project = fields.Boolean(string="Real-Estate Project")
    location = fields.Char(string="Location")
    developer_id = fields.Many2one("res.partner", string="Developer")
    delivery_date = fields.Date(string="Delivery Date")
    image_1920 = fields.Image(string="Project Image")
    currency_id = fields.Many2one(
        "res.currency",
        compute="_compute_currency_id",
        help="Company currency (kept live rather than frozen at creation).",
    )
    project_unit_ids = fields.One2many(
        "product.template", "categ_id", string="Units",
    )
    unit_count = fields.Integer(compute="_compute_unit_stats", string="Total Units")
    available_unit_count = fields.Integer(
        compute="_compute_unit_stats", string="Available Units",
    )
    price_from = fields.Monetary(
        compute="_compute_unit_stats",
        currency_field="currency_id",
        string="Starting Price",
    )

    def _compute_currency_id(self):
        company_currency = self.env.company.currency_id
        for categ in self:
            categ.currency_id = company_currency

    @api.depends("project_unit_ids.list_price", "project_unit_ids.unit_state")
    def _compute_unit_stats(self):
        for categ in self:
            units = categ.project_unit_ids
            categ.unit_count = len(units)
            categ.available_unit_count = len(
                units.filtered(lambda u: u.unit_state == "available")
            )
            prices = units.mapped("list_price")
            categ.price_from = min(prices) if prices else 0.0
