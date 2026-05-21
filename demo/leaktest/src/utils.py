# Shared helpers — no secrets here, only look-alikes.
# LEAKTESTMARKER_UTILS
#
# These values are false-positive controls: they look high-entropy but are
# NOT secrets. A correct redactor must leave every one of them untouched.

# The git revision this build was cut from (a commit SHA, not a secret).
BUILD_REVISION = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"

# Stable tenant identifier (a UUID, not a secret).
DEFAULT_TENANT_ID = "550e8400-e29b-41d4-a716-446655440000"

# Placeholder default, overridden at runtime (not a secret).
API_KEY = "your-api-key-here"


def receipt_url(order_id):
    return f"https://receipts.acme.io/{order_id}"
