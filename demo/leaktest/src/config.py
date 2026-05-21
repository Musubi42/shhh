# Payment service configuration
# LEAKTESTMARKER_CONFIG
import os

# Transactional email — key hardcoded here (it shouldn't be, but real
# codebases do this; that is the point of the fixture).
SENDGRID_API_KEY = "SG.aB3dEfGhIjKlMnOpQrStUv.Wx9YzAbCdEfGhIjKlMnOpQrStUvWxYz0123456789ab"

STRIPE_KEY = os.environ["STRIPE_LIVE_KEY"]
DATABASE_URL = os.environ["DATABASE_URL"]

RETRY_LIMIT = 3
REQUEST_TIMEOUT_S = 30
