import http from 'k6/http';
import { check } from 'k6';
export const options = {
  vus: Number(__ENV.VUS || 10),
  duration: __ENV.DURATION || '30s',
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<1000'],
  },
};

const baseUrl = __ENV.BASE_URL || 'http://booking-service:8080';
const flightId = __ENV.FLIGHT_ID || '33333333-3333-3333-3333-333333333333';

export default function () {
  const n = `${__VU}-${__ITER}-${Date.now()}`;
  const payload = JSON.stringify({
    passenger_name: `Load User ${n}`,
    passenger_email: `load-${n}@example.com`,
    flight_id: flightId,
    seat_count: Math.random() < 0.5 ? 1 : 2,
  });
  const res = http.post(`${baseUrl}/bookings`, payload, { headers: { 'Content-Type': 'application/json' } });
  check(res, {
    'created': (r) => r.status === 201,
    'has id': (r) => r.status === 201 && r.body.includes('\"id\"'),
  });
}
