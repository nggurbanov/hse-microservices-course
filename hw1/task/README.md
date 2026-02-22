# Маркетплейс — архитектура

## Что это

Проектируем архитектуру маркетплейса: продавцы выкладывают товары, покупатели смотрят ленту и заказывают. Нужно обеспечить персонализированную ленту, каталог, заказы, платежи, уведомления.

## Домены

Выделил 6 доменов:

| Домен | Что делает |
|-------|-----------|
| Users | Регистрация, авторизация, профили покупателей и продавцов |
| Catalog | CRUD товаров, цены, категории, остатки |
| Feed & Search | Персонализированная лента, поиск, рекомендации |
| Orders | Весь lifecycle заказа — от создания до завершения |
| Payments | Проведение оплаты, работа с платежными шлюзами, учет транзакций |
| Notifications | Email/push/SMS по событиям (заказ создан, оплачен, отправлен и т.д.) |

## Варианты разбиения

Рассмотрел два подхода:

### Вариант A — по домену на сервис (fine-grained)

Каждый домен = отдельный микросервис + своя БД:

```
API Gateway
├── User Service         → PostgreSQL
├── Catalog Service      → PostgreSQL
├── Feed Service         → Elasticsearch
├── Order Service        → PostgreSQL
├── Payment Service      → PostgreSQL
└── Notification Service → Redis + PostgreSQL
```

Между сервисами — Kafka для событий.

### Вариант Б — укрупнённый (coarse-grained)

Объединяем связанные домены:

```
API Gateway
├── Account Service       → PostgreSQL  (Users)
├── Commerce Service      → PostgreSQL  (Catalog + Orders + Payments)
├── Discovery Service     → Elasticsearch  (Feed & Search)
└── Notification Service  → Redis
```

## Сравнение

| | Вариант А | Вариант Б |
|-|-----------|-----------|
| Деплой | Каждый сервис отдельно | Commerce — 3 домена за раз |
| Масштабирование | Точечное (например, отдельно Feed) | Commerce целиком, даже если bottleneck только в каталоге |
| Инфраструктура | Сложнее — 6 сервисов, 6 БД, больше мониторинга | Проще — 4 сервиса |
| Задержки | Больше сетевых вызовов между сервисами | Catalog/Order/Payment общаются внутри процесса |
| Консистентность | Нужны Saga-паттерны для распределённых транзакций | ACID в рамках одной БД для Commerce |
| Команды | Полная автономность | Команда Commerce координируется внутри |
| Время на старте | Больше boilerplate | Быстрее стартовать |
| Дальнейшее развитие | Легко добавить новый сервис | Commerce может превратиться в мини-монолит |

## Почему выбрал вариант А

1. **Feed нагружен сильнее всего** — лента товаров read-heavy, её нужно масштабировать независимо от остальных.
2. **Деплой без рисков** — обновление платёжки не трогает каталог. В варианте Б изменение в Commerce = регресс по трём доменам.
3. **Изоляция данных** — каждый сервис со своей БД, никто не лезет в чужие таблицы напрямую. В Б три домена делят одну базу.
4. **CQRS** — Catalog пишет, Feed читает из Elasticsearch. Это классическое разделение чтения и записи, которое ложится на fine-grained декомпозицию.
5. **На будущее** — проще начать с чётких границ, чем потом выпиливать домены из разросшегося сервиса.

## Владение данными

| Сервис | Данные | Хранилище |
|--------|--------|-----------|
| User Service | Профили, аккаунты, роли, токены | PostgreSQL |
| Catalog Service | Товары, категории, цены, остатки | PostgreSQL |
| Feed Service | Поисковый индекс товаров, веса персонализации | Elasticsearch |
| Order Service | Заказы, позиции, статусы | PostgreSQL |
| Payment Service | Транзакции, чеки, статусы оплат | PostgreSQL |
| Notification Service | Шаблоны, очередь, логи | Redis + PostgreSQL |

Каждый сервис владеет своей БД. Доступ к чужим данным — только через API или события.

## Взаимодействие сервисов

```
              Покупатель / Продавец
                       │
                       ▼
                  API Gateway ──► Redis (кэш)
                       │
         ┌─────────────┼──────────────┐
         ▼             ▼              ▼
   User Service   Catalog Service   Feed Service
    (Postgres)     (Postgres)      (Elasticsearch)
         │             │    ▲              ▲
         │             │    │ gRPC         │ Kafka
         │             ▼    │         (product_events)
         │        Order Service
         │         (Postgres)
         │             │
         │             │ Kafka (order_events)
         │             ▼
         │      Payment Service ──► Платёжный шлюз
         │         (Postgres)
         │             │ Kafka (payment_events)
         │             ▼
         └────► Notification Service
                 (Redis + Postgres)
```

### Синхронные вызовы (REST/gRPC)

- API Gateway → все сервисы (маршрутизация запросов клиента)
- Order Service → Catalog Service (проверка наличия, резерв товара) — gRPC
- Order Service → User Service (данные покупателя) — gRPC

### Асинхронные события (Kafka)

| Кто шлёт | Топик | Кто слушает | Суть |
|-----------|-------|-------------|------|
| Catalog Service | `product_events` | Feed Service | Товар создан/обновлён — обновить индекс |
| Order Service | `order_events` | Payment, Notification | Заказ создан/изменён |
| Payment Service | `payment_events` | Order, Notification | Результат оплаты |
| User Service | `user_events` | Feed Service | Просмотры, клики — для персонализации |

## Персонализация

Feed Service строит персонализированную ленту так:
- Собирает `user_events` (что пользователь смотрел, кликал, покупал) и формирует вектор предпочтений по категориям
- При запросе ленты — boost в Elasticsearch по этим категориям, товары из «интересных» категорий ранжируются выше
- Опционально — collaborative filtering (показывать то, что покупали похожие пользователи)

## C4 Container диаграмма

Файл: [`marketplace.c4`](marketplace.c4)

Содержит: двух акторов (покупатель, продавец), API Gateway, 6 микросервисов, хранилища (4× PostgreSQL, Elasticsearch, Redis), Kafka. Все связи подписаны.

## Реализованный сервис

Для демонстрации поднят **Catalog Service** — Python + FastAPI. На этом этапе без бизнес-логики, только health-check.

```
catalog_service/
├── main.py
├── requirements.txt
└── Dockerfile
docker-compose.yml
```

## Запуск

```bash
cd hw1/task
docker compose up --build -d

# проверка
curl http://localhost:8000/health
# → {"status": "ok"}

# остановка
docker compose down
```

Swagger UI: http://localhost:8000/docs
