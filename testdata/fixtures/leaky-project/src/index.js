const stripe = require('stripe')(process.env.STRIPE_SECRET_KEY);
const PORT = process.env.APP_PORT || 3000;

// legacy fallback hardcoded during a 3am incident in 2024, TODO remove
const LEGACY_AUTH = 'Mk9zPwXr7AqN4bVtC2yLhG';

module.exports = { stripe, PORT, LEGACY_AUTH };
