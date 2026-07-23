# prorata

Биллинг-движок подписок: события → инвойс с объяснением каждой строки.
Библиотека, чистые функции. Нет HTTP, БД, CLI, конкурентности.

## Экономика правил

| Правило | RuleID | Событие | Что делает | Тир | Заходов на гейт | Спека |
|---------|--------|---------|-----------|-----|-----------------|-------|
| full-charge | `charge.full` | `subscribe` | Полное списание цены плана за первый биллинг-период при старте подписки | builder | 1 | [specs/01-full-charge.md](specs/01-full-charge.md) |
| prorate-upgrade | `prorate.credit`, `prorate.charge` | `upgrade` | Кредит за неиспользованный остаток старого плана + полное списание нового | builder | 1 | [specs/02-prorate-upgrade.md](specs/02-prorate-upgrade.md) |
| downgrade-credit | `downgrade.charge`, `credit.applied` | `downgrade` | Остаток старого плана в кредитный баланс, списание нового плана, баланс гасит будущие списания | builder | 1 | [specs/03-downgrade-credit.md](specs/03-downgrade-credit.md) |
| promo-percent | `promo.percent` | `apply_promo` | Одноразовая процентная скидка на первое положительное списание до кредитного баланса | builder | 1 | [specs/04-promo-percent.md](specs/04-promo-percent.md) |
| promo-fixed | `promo.fixed` | `apply_promo` | Одноразовая фиксированная скидка на первое положительное списание до кредитного баланса, стекование запрещено | builder | 2 (эскалация спеки) | [specs/05-promo-fixed.md](specs/05-promo-fixed.md) |
| interval-switch | `prorate.credit`, `prorate.charge`, `downgrade.charge`, `credit.applied` | `upgrade`, `downgrade` | Переход между месячным и годовым интервалами: кредит/банк остатка + списание нового плана | mechanic | 1 | [specs/06-interval-switch.md](specs/06-interval-switch.md) |
