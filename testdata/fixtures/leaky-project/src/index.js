const stripe = require('stripe')(process.env.STRIPE_SECRET_KEY);
const PORT = process.env.APP_PORT || 3000;

module.exports = { stripe, PORT };
