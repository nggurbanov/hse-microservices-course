# HW7 Flight Booking: CI/CD, Testing & Observability

Это развитие системы из ДЗ3: `booking-service` принимает REST-запросы, `flight-service` обслуживает gRPC, у каждого сервиса своя PostgreSQL, flight дополнительно использует Redis cache-aside.

## Запуск

```bash
cd hw7
docker compose up --build
```

Интерфейсы:

- Booking API: http://localhost:8080
- Booking metrics: http://localhost:8080/metrics
- Flight metrics/health: http://localhost:9090/metrics
- Prometheus: http://localhost:9091
- Grafana: http://localhost:3000 (`admin` / `admin`)
- Alertmanager: http://localhost:9093

Тестовый рейс для E2E: `11111111-1111-1111-1111-111111111111`.
Рейс для нагрузки: `33333333-3333-3333-3333-333333333333`.

## CI pipeline

GitHub Actions workflow: `.github/workflows/hw7-ci.yml`.

Пайплайн запускается на push/PR и выполняет:

1. `go test ./...`
2. `docker compose build`
3. `docker compose up -d`
4. integration test
5. E2E test
6. k6 load test
7. Prometheus SLI check
8. загрузку логов и результатов как artifacts

Локально эти шаги можно повторить одной командой:

```bash
cd hw7
./scripts/ci_local.sh
```

Или по шагам:

```bash
cd hw7
go test ./...
docker compose build
docker compose up -d
./scripts/integration_test.sh
./scripts/e2e_test.sh
./scripts/load_test.sh
./scripts/check_prometheus_sli.sh
```

## Тестовые сценарии

Integration test проверяет межсервисное взаимодействие:

- `booking-service` получает HTTP-запрос на создание бронирования;
- вызывает `flight-service` по gRPC;
- в `flight-db` появляется `seat_reservations`;
- в `booking-db` появляется `bookings`;
- `available_seats` уменьшается;
- overbooking возвращает ошибку и не создаёт booking.

E2E test проверяет пользовательский сценарий:

- создать бронирование;
- проверить HTTP-ответ и записи в двух БД;
- отменить бронирование;
- проверить `CANCELLED`, `RELEASED` и возврат мест.

## Метрики

`booking-service` отдаёт HTTP-метрики:

- `http_requests_total{method, endpoint, status}`
- `http_request_errors_total{method, endpoint, error_type}`
- `http_request_duration_seconds{method, endpoint}`

`flight-service` отдаёт gRPC-метрики:

- `grpc_requests_total{method, endpoint, status}`
- `grpc_request_errors_total{method, endpoint, error_type}`
- `grpc_request_duration_seconds{method, endpoint}`

Prometheus также собирает инфраструктуру через exporters:

- PostgreSQL exporter для booking DB;
- PostgreSQL exporter для flight DB;
- Redis exporter.

## Grafana dashboards

Grafana автоматически поднимает datasource и два dashboard из JSON:

- `HW7 Services`: throughput, latency p50/p95/p99 и error rate для booking и flight;
- `HW7 Infrastructure`: Prometheus targets, PostgreSQL connections/transactions, Redis memory/ops.

## Нагрузочный тест

k6 запускается через Docker:

```bash
cd hw7
./scripts/load_test.sh
```

Параметры по умолчанию:

- 10 VU;
- 30 секунд;
- threshold `http_req_failed < 1%`;
- threshold `p95 latency < 1000ms`.

Нагрузка идёт на отдельный seeded flight `LOAD1`, чтобы тест не упирался в количество мест. Результаты сохраняются в `artifacts/k6-summary.json` и `artifacts/k6.log`.

## Alert rules

Alert rules лежат в `observability/prometheus/alerts.yml`.

Алерты:

- `HighErrorRate`: больше 5% HTTP-ошибок за 30 секунд;
- `ServiceDown`: Prometheus не может scrape `booking-service` или `flight-service` 30 секунд.

Показать `HighErrorRate`:

```bash
cd hw7
./scripts/fire_error_alert.sh
```

Через ~30 секунд открыть:

- http://localhost:9091/alerts
- http://localhost:9093

Показать `ServiceDown`:

```bash
cd hw7
docker compose stop booking-service
```

После демонстрации:

```bash
docker compose start booking-service
```

## SLI/SLO

| SLI | PromQL | SLO | Failure threshold |
|---|---|---:|---:|
| API availability | `sum(rate(http_requests_total{endpoint="/bookings",status!~"5.."}[1m])) / clamp_min(sum(rate(http_requests_total{endpoint="/bookings"}[1m])), 1)` | > 99% | < 95% |
| API p95 latency | `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket{endpoint="/bookings"}[1m])))` | < 500ms | > 1000ms |
| gRPC dependency success | `sum(rate(grpc_requests_total{status!~"Internal|Unavailable|DeadlineExceeded|Unknown"}[1m])) / clamp_min(sum(rate(grpc_requests_total[1m])), 1)` | > 99% | < 95% |

В CI используется `scripts/check_prometheus_sli.sh`. Он проверяет метрики из Prometheus API после нагрузки по 15-минутному окну, чтобы не зависеть от конкретной секунды scrape, и падает, если:

- 5xx system error rate `>= 1%`;
- p95 latency `>= 1000ms`;
- availability `< 99%`;
- gRPC success `< 99%`.
