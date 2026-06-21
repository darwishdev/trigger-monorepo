from odoo import fields, models


class ProductTemplate(models.Model):
    """A product template doubles as a sellable real-estate Unit.

    The unit belongs to its project through the standard categ_id field.
    """

    _inherit = "product.template"

    is_property = fields.Boolean(string="Property Unit")
    unit_code = fields.Char(string="Unit Code")
    unit_state = fields.Selection(
        [
            ("available", "Available"),
            ("reserved", "Reserved"),
            ("sold", "Sold"),
        ],
        string="Unit Status",
        default="available",
    )
    property_type = fields.Selection(
        [
            ("apartment", "Apartment"),
            ("villa", "Villa"),
            ("townhouse", "Townhouse"),
            ("office", "Office"),
            ("retail", "Retail"),
        ],
        string="Property Type",
    )
    area_sqm = fields.Float(string="Area (m²)")
    bedrooms = fields.Integer(string="Bedrooms")
    bathrooms = fields.Integer(string="Bathrooms")
    floor = fields.Char(string="Floor")
