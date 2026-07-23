# 01-full-charge — полное списание периода при подписке

Тир спеки: architect. Реализация: builder. Фикстуры/тесты: mechanic.

## Что делает правило

Обрабатывает `EventSubscribe`: старт подписки на план с полным списанием цены за первый биллинг-период.

Поведение (по шагам):
1. План берётся из каталога по `Event.PlanID`. Неизвестный план → ошибка `prorata: subscribe: unknown plan "<id>"`.
2. Если подписка уже активна (state.plan != nil) → ошибка `prorata: subscribe: already subscribed`.
3. Биллинг-период подписки: `[ev.At, AddInterval(ev.At, plan.Interval))` — кламп дня по D3 из specs/00-core.md.
4. Состояние: plan, periodStart, periodEnd, paid = plan.Price.
5. Строка инвойса (одна):
   - `RuleID`: `charge.full`
   - `Description`: `<plan.ID>: full period <start> to <end>`, даты в UTC формата `2006-01-02`
   - `Amount`: `plan.Price`

Напоминание из ядра: строку в инвойс включает движок только если событие внутри запрошенного периода; правило строку возвращает всегда.

## Краевые кейсы

- Подписка 31-го числа: конец месячного периода клампится (31 янв → 28/29 фев).
- Событие до запрошенного периода: состояние строится, строки в инвойсе нет (пустой инвойс: `lines` отсутствует, `total_cents: 0`).
- Событие ровно в `period.End` запрошенного периода — НЕ входит (полуинтервал).
- Повторный subscribe при активной подписке — ошибка (см. выше), не вторая строка.
- Даты в Description — всегда UTC (`t.UTC().Format("2006-01-02")`).

## Примеры вход→выход (точные значения — фикстуры строить байт-в-байт)

Общий каталог примеров:

| id | price_cents | interval | currency |
|---|---|---|---|
| pro-month | 2000 | month | USD |
| business-year | 48000 | year | USD |

### Пример 1 → golden/01-monthly-subscribe.json

Вход: событие `subscribe pro-month` в `2026-01-01T00:00:00Z`; запрошенный период `2026-01-01T00:00:00Z` → `2026-02-01T00:00:00Z`.

Выход:
```json
{
  "currency": "USD",
  "lines": [
    {"rule": "charge.full", "description": "pro-month: full period 2026-01-01 to 2026-02-01", "amount_cents": 2000}
  ],
  "total_cents": 2000
}
```

### Пример 2 → golden/01-yearly-subscribe.json

Вход: событие `subscribe business-year` в `2026-01-13T00:00:00Z`; запрошенный период `2026-01-01T00:00:00Z` → `2026-02-01T00:00:00Z`.

Выход:
```json
{
  "currency": "USD",
  "lines": [
    {"rule": "charge.full", "description": "business-year: full period 2026-01-13 to 2027-01-13", "amount_cents": 48000}
  ],
  "total_cents": 48000
}
```

### Пример 3 → golden/01-subscribe-clamp-jan31.json

Вход: событие `subscribe pro-month` в `2026-01-31T00:00:00Z`; запрошенный период `2026-01-01T00:00:00Z` → `2026-02-01T00:00:00Z`.

Выход:
```json
{
  "currency": "USD",
  "lines": [
    {"rule": "charge.full", "description": "pro-month: full period 2026-01-31 to 2026-02-28", "amount_cents": 2000}
  ],
  "total_cents": 2000
}
```

### Пример 4 → golden/01-subscribe-before-period.json

Вход: событие `subscribe pro-month` в `2026-01-01T00:00:00Z`; запрошенный период `2026-02-01T00:00:00Z` → `2026-03-01T00:00:00Z`.

Выход:
```json
{
  "currency": "USD",
  "total_cents": 0
}
```

### Формат golden-файла (как в golden_test.go)

```json
{
  "name": "<имя сценария>",
  "catalog": [{"id": "pro-month", "price_cents": 2000, "interval": "month", "currency": "USD"}],
  "events": [{"at": "2026-01-01T00:00:00Z", "type": "subscribe", "plan": "pro-month"}],
  "period": {"start": "2026-01-01T00:00:00Z", "end": "2026-02-01T00:00:00Z"},
  "invoice": { ... как в примерах выше ... }
}
```

## Табличные тесты (rule_charge_test.go, пишет mechanic)

Через публичный `Compute` (каталог из примеров):

1. неизвестный план: событие `subscribe ghost` → ошибка (err != nil, инвойс не проверять);
2. повторный subscribe: два события `subscribe pro-month` (2026-01-01 и 2026-01-05) → ошибка;
3. событие ровно в End запрошенного периода: subscribe в `2026-02-01T00:00:00Z`, период `2026-01-01` → `2026-02-01` → инвойс без строк, total 0.

## Gate

(заполнит architect после реализации)
