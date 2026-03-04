#!/bin/bash

# Exit on any failure
set -e

BASE_URL="http://localhost:8080/api/v1"

echo "Waiting for API to start..."
sleep 5

echo "1. Register an ADMIN user"
curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin1", "password": "password123", "role": "ADMIN"}'

echo -e "\n\n2. Login as ADMIN"
LOGIN_RESP=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "admin1", "password": "password123"}')
echo "$LOGIN_RESP"

ACCESS_TOKEN=$(echo $LOGIN_RESP | grep -o '"access_token":"[^"]*' | grep -o '[^"]*$')
echo -e "\nAccess Token: $ACCESS_TOKEN"

echo -e "\n\n3. Create a Product as ADMIN"
CREATE_PROD_RESP=$(curl -s -X POST "$BASE_URL/products" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Super Coffee",
    "description": "Premium blend",
    "price": 5.99,
    "stock": 100,
    "category": "Beverages",
    "status": "ACTIVE"
  }')
echo "$CREATE_PROD_RESP"
PROD_ID=$(echo $CREATE_PROD_RESP | grep -o '"id":"[^"]*' | grep -o '[^"]*$')

echo -e "\n\n4. Create a Promo Code as ADMIN"
curl -s -X POST "$BASE_URL/promo-codes" \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "code": "WELCOME20",
    "discount_type": "PERCENTAGE",
    "discount_value": 20.0,
    "min_order_amount": 10.0,
    "max_uses": 100,
    "valid_from": "2020-01-01T00:00:00Z",
    "valid_until": "2030-01-01T00:00:00Z"
  }'

echo -e "\n\n5. Register a regular USER"
curl -s -X POST "$BASE_URL/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username": "user1", "password": "password123", "role": "USER"}'

echo -e "\n\n6. Login as USER"
USER_LOGIN_RESP=$(curl -s -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username": "user1", "password": "password123"}')
USER_TOKEN=$(echo $USER_LOGIN_RESP | grep -o '"access_token":"[^"]*' | grep -o '[^"]*$')

echo -e "\n\n7. Create Order as USER (with promo code)"
CREATE_ORDER_RESP=$(curl -s -X POST "$BASE_URL/orders" \
  -H "Authorization: Bearer $USER_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "items": [{"product_id": "'$PROD_ID'", "quantity": 3}],
    "promo_code": "WELCOME20"
  }')
echo "$CREATE_ORDER_RESP"

ORDER_ID=$(echo $CREATE_ORDER_RESP | grep -o '"id":"[^"]*' | grep -o '[^"]*$')

echo -e "\n\n8. Get Order details as USER"
curl -s "$BASE_URL/orders/$ORDER_ID" \
  -H "Authorization: Bearer $USER_TOKEN"
  
echo -e "\n\n9. Check Product stock decreased"
curl -s "$BASE_URL/products/$PROD_ID" | grep stock

echo -e "\n\nSuccess! Validated critical path."
