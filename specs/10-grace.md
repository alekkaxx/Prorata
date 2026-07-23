# 10-grace — grace period при неоплате

Тир: architect. Правило №11 бэклога PLAN. Формат решений: **решение → почему → отвергнутая альтернатива**.
События: `EventPaymentFailed` (`payment_failed`), `EventGraceRecover` (`grace_recover`),
`EventGraceExpire` (`grace_expire`). RuleID: `grace.start` | `grace.recover` | `grace.expire`.

Ядро подготовлено architect'ом (коммит `core(grace): ...`): три типа события + поля
`state.grace bool`, `state.graceHeldCash Money`; teardown рефанда чистит grace. Хуки
`applyPromo`/`applyVAT`/`applyCredit` **не менялись** — все взаимодействия эмерджентны.

Builder пишет **только** `rule_grace.go` — три обработчика, регистрируемых через
`registerRule` в `init()`. Ядро не трогать. RuleID грейса объявляются в `rule_grace.go`
(как `trial.start`/`refund.*` в своих файлах), в ядре их нет.

## Что делает правило

Грейс — это «неоплата не рвёт подписку сразу». Когда списание за текущий период не
собралось (карта отклонена), доступ сохраняется, а вызывающая сторона позже сообщает
исход: платёж дошёл (`grace_recover`) или дунинг сдался (`grace_expire`). Грейс сам по
себе **денег не двигает** — все три строки нулевые; единственная сумма на инвойсе за
период — исходная строка списания.

**Фаза 1 — неоплата (`EventPaymentFailed`):**
1. `st.plan == nil` → ошибка `prorata: grace: no active subscription`.
2. `st.trial` → ошибка `prorata: grace: cannot fail payment on a trial` (у триала нечему
   не оплатиться, `cashPaid == 0`; см. D5).
3. `st.grace` → ошибка `prorata: grace: already in grace`.
4. `ev.PlanID` нет в каталоге → ошибка `prorata: grace: unknown plan "<id>"` (well-formedness,
   как refund D7; расчёт идёт от `st.plan`, не от события).
5. Состояние: `graceHeldCash = cashPaid`; `paid = 0`; `cashPaid = 0`; `grace = true`.
   Подписка **жива** (`plan`, `periodStart/End` не трогаются).
6. Строка (одна, **нулевая**): `grace.start`, `Amount 0`,
   `Description = "<st.plan.ID>: payment failed <ev.At>, access retained"`.

**Фаза 2а — платёж восстановлен (`EventGraceRecover`):**
1. `!st.grace` → ошибка `prorata: grace: no grace in progress`.
2. `ev.PlanID` нет в каталоге → ошибка `prorata: grace: unknown plan "<id>"`.
3. Состояние: `paid = st.plan.Price`; `cashPaid = graceHeldCash`; `graceHeldCash = 0`;
   `grace = false`. Нового заряда нет (исходное списание уже стоит).
4. Строка: `grace.recover`, `Amount 0`,
   `Description = "<st.plan.ID>: payment recovered <ev.At>, subscription continues"`.

**Фаза 2б — грейс истёк (`EventGraceExpire`):**
1. `!st.grace` → ошибка `prorata: grace: no grace in progress`.
2. `ev.PlanID` нет в каталоге → ошибка `prorata: grace: unknown plan "<id>"`.
3. Запомнить `planID := st.plan.ID` **до** teardown.
4. Teardown подписки: `plan = nil`, `periodStart/End = zero`, `paid = 0`, `cashPaid = 0`,
   `trial = false`, `grace = false`, `graceHeldCash = 0`. **Леджер не трогается**
   (`creditBalance`, вооружённый `promo` сохраняются — как refund D5).
5. Строка: `grace.expire`, `Amount 0`,
   `Description = "<planID>: grace expired <ev.At>, access ended for non-payment"`.

Даты — UTC, формат `2006-01-02`, единственная дата = `ev.At`. Напоминание ядра: строку в
инвойс включает движок только если событие внутри запрошенного периода; правило строку
возвращает всегда.

---

## Ключевые решения

### D1. Модель неоплаты — событие от вызывающего, часов в библиотеке нет

**Решение:** неоплату сообщает событие `EventPaymentFailed`, исход грейса — тоже события
(`grace_recover`/`grace_expire`). Библиотека не решает «пришла ли оплата» и «истёк ли
грейс» — это делает шедулер/PSP вызывающего.

**Почему:** движок — чистая функция без часов и таймеров (D4 ядра, D7 правила 07). «Оплата
не пришла» и «грейс кончился» — внешние факты; выразить их можно только событием на входе.
Три фазы = три события зеркалят двухфазную модель триала (07 D3): каждое состояние-переход
имеет своё событие с явной семантикой.

**Отвергнуто:** авто-обрыв грейса по `periodEnd + N` внутри `Compute` — потребовал бы
«текущего времени», убил бы чистоту и идемпотентность (ровно 07 D7). Флаг неоплаты на плане
— неоплата не свойство тарифа, а разовый факт биллинга.

### D2. Длина грейса N — не в модели вовсе

**Решение:** длительности грейса в библиотеке **нет** — ни поля `GraceDays` у плана, ни у
события. Границу грейса задаёт вызывающий, эмитя `grace_recover`/`grace_expire` тогда, когда
его политика (N дней) решит.

**Почему:** без часов длина N ничего не «отсчитывает» внутри движка — она живёт только в
шедулере вызывающего. Хранить N в модели значило бы добавить поле, которое никто не читает.
«Первый период бесплатно» триала выражается нулём кода (07 D1) — так же и грейс: исход даёт
событие, а не отсчёт.

**Отвергнуто:** `Plan.GraceDays` или `Event`-поле длины — расширяет модель ради числа, которое
движок не использует (нет клока, чтобы его применить); дырка в скоупе «библиотека без часов».

### D3. Грейс денег не двигает — три нулевые строки-маркеры

**Решение:** все три строки грейса нулевые (`Amount 0`) с RuleID и Description. Единственная
денежная строка за период — исходное списание (`charge.full`/`prorate.charge`/…).

**Почему:** инвойс обязан объяснять доступ, а не молчать (объяснимость PLAN, ровно 07 D2):
`grace.start` фиксирует «доступ сохранён при неоплате», `grace.recover` — «оплата дошла»,
`grace.expire` — «доступ закрыт за неоплату». Движок пропускает нулевой `Amount` (проверка
только на пустые RuleID/Description), деньги не создаются (`Total` слагаемых 0).

**Отвергнуто:** отсутствие строк (грейс молча меняет состояние) — зритель не видит, почему
доступ был/пропал; теряется след правила в инвойсе.

### D4. При истечении грейса заряд НЕ реверсится — долг остаётся

**Решение:** `grace_expire` только сворачивает подписку. Несобранный заряд (напр. `charge.full`
2000) **остаётся** строкой на инвойсе как непогашенный долг; реверсной/кредитной строки нет.

**Почему:** во время грейса `cashPaid == 0` (кэш не собран). Реверс-строка `−2000` была бы
**кредитом, превышающим фактически собранное (0)** — прямое нарушение инварианта «кредит ≤
уплаченного» и красных property-тестов (`credit.applied`/`prorate.credit ≤ paid`). Неоплаченный
инвойс так и остаётся к оплате после обрыва доступа — это корректный биллинг; списание в
безнадёжный долг — отдельное бухгалтерское действие вне скоупа (PSP/леджера долгов у нас нет).

**Отвергнуто:** «void» несобранного заряда строкой `−cashOwed` — создаёт кредит > собранного
кэша, ломает главный инвариант проекта. Держать долг честной строкой безопаснее и правдивее.

### D5. `payment_failed` зануляет paid и cashPaid — честный «кэш не собран»

**Решение:** фаза 1 ставит `paid = 0`, `cashPaid = 0` (пред-грейсовый `cashPaid` → `graceHeldCash`).

**Почему:** это делает **все** взаимодействия во время грейса честными **без единой строчки в
чужих правилах** — ровно эмерджентность проекта (07 D6, «поведение из уже загейченного хука»):
- **refund во время грейса:** refund считает от `cashPaid` (08 D3); при `cashPaid == 0` любая
  политика возвращает **0.00** — нельзя вернуть кэш, который не собрали (пример 3). Ни строки в
  `rule_refund.go`.
- **upgrade/downgrade во время грейса:** proration/банк считают от `paid`; при `paid == 0`
  кредит остатка = 0 — нельзя кредитовать список за неоплаченный период. Ни строки в
  `rule_prorate.go`/`rule_downgrade.go`.
- `grace_recover` восстанавливает `cashPaid = graceHeldCash` (собранный кэш = то, что было
  должно), `paid = st.plan.Price` (для активного периода `paid` всегда `== plan.Price` во всех
  заряжающих правилах). Триал сюда не попадает — фаза 1 его отвергает.

**Отвергнуто:** не трогать `cashPaid`/`paid`, а учить `refund`/`prorate` смотреть на `st.grace` —
размазывает грейс-логику по builder-правилам, расширяет коммит, ломает симметрию «инвариант
держит ядро/хук». Хранить только `cashPaid` и восстанавливать `paid = plan.Price` без
`graceHeldCash` для cashPaid — `cashPaid` может быть < `paid` (промо/кредит уменьшили счёт),
поэтому его точное значение надо запомнить, иначе refund после recover вернёт больше собранного.

### D6. Teardown рефанда чистит grace — держит «grace ⇒ plan != nil»

**Решение:** teardown в `rule_refund.go` теперь ставит `grace = false`, `graceHeldCash = 0`
(рядом с уже существующим `trial = false`).

**Почему:** refund во время грейса тоже сворачивает подписку. Не почистив флаг, получили бы
`grace == true` при `plan == nil`; последующая переподписка унаследовала бы «висящий» грейс, и
новый `payment_failed` упал бы «already in grace». Флаг чистится → инвариант **«grace ⇒ plan !=
null»** держится, а значит `grace_recover`/`grace_expire` при `grace == true` всегда видят живой
`st.plan` (нет nil-разыменования, отдельный guard `plan != nil` не нужен). Это правка того же
класса, что уже существующий сброс `trial` в teardown.

**Отвергнуто:** guard `plan != nil` в recover/expire вместо чистки флага — прячет симптом,
оставляет висящий грейс, ломающий переподписку. Общий teardown-хелпер — запрещён (helpers.go).

### D7. promo, VAT, creditBalance — без спец-логики (нулевой `base`/`net`)

**Решение:** `rule_grace.go` не трогает хуки. Нулевые строки грейса дают `base = 0` (promo),
`net = 0` (VAT и credit).

**Почему:** `applyPromo` при `base ≤ 0` пропускает и **оставляет промо вооружённым** — акция
переживает грейс и сработает на следующем реальном заряде (ровно 04 D3, 07 D6). `applyVAT`/
`applyCredit` при `net ≤ 0` не порождают строк — грейс не облагается VAT и не гасит баланс.
`creditBalance` грейс переживает (leджер, как refund D5). Всё — эмерджентно, ноль нового кода в
ядре.

**Отвергнуто:** явно жечь/сохранять promo или трогать баланс в правиле грейса — дублирует хуки,
ломает симметрию «скидку/кредит применяет только движок».

---

## Краевые кейсы

- **`payment_failed` без подписки** (`plan == nil`) → `grace: no active subscription`.
- **`payment_failed` на триале** (`trial == true`) → `grace: cannot fail payment on a trial`.
- **Двойной `payment_failed`** → второй видит `grace == true` → `grace: already in grace`.
- **`grace_recover`/`grace_expire` без открытого грейса** (пустое состояние, платная подписка
  без грейса, после recover/expire) → `grace: no grace in progress`.
- **Неизвестный план** в любом из трёх событий → `grace: unknown plan "<id>"`.
- **refund во время грейса** → `cashPaid == 0` → возврат `0.00` любой политикой; teardown
  чистит грейс (D6), подписка закрыта (пример 3).
- **recover → refund** → `cashPaid` восстановлен, refund честно возвращает собранное (пример 4).
- **expire → resubscribe** → грейс очищен teardown'ом, новая подписка стартует чисто; долг
  прошлого периода остаётся строкой в истории (D4).
- **`payment_failed` на $0/полностью-кредитном периоде** (`cashPaid == 0`, `trial == false`) —
  легально: маркеры нулевые, recover/expire восстанавливают/зануляют 0; не ошибка.
- **Плановые изменения (upgrade/downgrade) во время открытого грейса** — вне скоупа правила
  11: штатный жизненный цикл грейса `failed → recover|expire`. При `paid == 0` proration в
  любом случае кредитует 0 (D5), но переопределение периода новым планом и «висящий» грейс —
  комбинация, которую вызывающий не порождает; граница отмечена намеренно.

## Инварианты (обязаны держаться)

- Деньги не создаются: все строки грейса = 0; несобранный заряд не реверсится (D4).
- Кредит ≤ уплаченного: во время грейса `paid = cashPaid = 0` → любой кредит/возврат = 0 (D5).
- `grace ⇒ plan != nil` (D6): teardown'ы чистят флаг.
- Каждая строка несёт RuleID и Description (нулевые маркеры — тоже).
- Идемпотентность: грейс — чистая функция состояния; повтор входа → тот же инвойс.

## Примеры (вход → выход, точные центы)

Каталог всех примеров: `pro-month` = 2000, month, USD. Период инвойса всюду
`[2026-01-01T00:00:00Z, 2026-03-01T00:00:00Z)`.

### Пример 1 → golden/10-grace-recover.json — неоплата, платёж восстановлен

События:
- `subscribe pro-month` @ 2026-01-01
- `payment_failed pro-month` @ 2026-01-05
- `grace_recover pro-month` @ 2026-01-08

`cashPaid`: 2000 → 0 (failed) → 2000 (recover). Подписка цела.

| rule | description | amount_cents |
|---|---|---|
| charge.full | `pro-month: full period 2026-01-01 to 2026-02-01` | 2000 |
| grace.start | `pro-month: payment failed 2026-01-05, access retained` | 0 |
| grace.recover | `pro-month: payment recovered 2026-01-08, subscription continues` | 0 |

**total_cents = 2000.**

### Пример 2 → golden/10-grace-expire.json — неоплата, грейс истёк

События:
- `subscribe pro-month` @ 2026-01-01
- `payment_failed pro-month` @ 2026-01-05
- `grace_expire pro-month` @ 2026-01-20

Подписка свёрнута; несобранные 2000 остаются долгом-строкой, реверса нет (D4).

| rule | description | amount_cents |
|---|---|---|
| charge.full | `pro-month: full period 2026-01-01 to 2026-02-01` | 2000 |
| grace.start | `pro-month: payment failed 2026-01-05, access retained` | 0 |
| grace.expire | `pro-month: grace expired 2026-01-20, access ended for non-payment` | 0 |

**total_cents = 2000.**

### Пример 3 → golden/10-grace-refund-zero.json — refund во время грейса возвращает 0

События:
- `subscribe pro-month` @ 2026-01-01
- `payment_failed pro-month` @ 2026-01-05
- `refund full` @ 2026-01-10

`cashPaid == 0` на момент рефанда → `refund.full` возвращает **0.00**, а не номинал 2000
(иначе создали бы деньги). Teardown чистит грейс (D6).

| rule | description | amount_cents |
|---|---|---|
| charge.full | `pro-month: full period 2026-01-01 to 2026-02-01` | 2000 |
| grace.start | `pro-month: payment failed 2026-01-05, access retained` | 0 |
| refund.full | `pro-month: full refund 2026-01-01 to 2026-02-01` | 0 |

**total_cents = 2000.** Ключевой кейс «refund во время грейса»: возвращать нечего.

### Пример 4 → golden/10-grace-recover-then-refund.json — recover честно восстановил cashPaid

События:
- `subscribe pro-month` @ 2026-01-01
- `payment_failed pro-month` @ 2026-01-05
- `grace_recover pro-month` @ 2026-01-08
- `refund prorated` @ 2026-01-13

После recover `cashPaid = 2000`. Refund prorated @ 01-13: период [01-01, 02-01) = 31 дн.,
`rem = Days(01-13, 02-01) = 19`, `used = 12`, `Allocate(2000, [12, 19]) = [774, 1226]` →
`unused = 1226`.

| rule | description | amount_cents |
|---|---|---|
| charge.full | `pro-month: full period 2026-01-01 to 2026-02-01` | 2000 |
| grace.start | `pro-month: payment failed 2026-01-05, access retained` | 0 |
| grace.recover | `pro-month: payment recovered 2026-01-08, subscription continues` | 0 |
| refund.prorated | `pro-month: refund unused 19/31 days 2026-01-13 to 2026-02-01` | -1226 |

**total_cents = 774.** Контраст с примером 3: recover вернул `cashPaid` к 2000, поэтому
refund возвращает 1226 (≤ 2000 собранного), а не 0.

### Формат golden-файла (как в golden_test.go)

```json
{
  "name": "10-grace-recover",
  "catalog": [
    {"id": "pro-month", "price_cents": 2000, "interval": "month", "currency": "USD"}
  ],
  "events": [
    {"at": "2026-01-01T00:00:00Z", "type": "subscribe", "plan": "pro-month"},
    {"at": "2026-01-05T00:00:00Z", "type": "payment_failed", "plan": "pro-month"},
    {"at": "2026-01-08T00:00:00Z", "type": "grace_recover", "plan": "pro-month"}
  ],
  "period": {"start": "2026-01-01T00:00:00Z", "end": "2026-03-01T00:00:00Z"},
  "invoice": { ... как в примерах выше ... }
}
```

Рефанд-событие в JSON: `{"at": "...", "type": "refund", "policy": "full"}` (без `plan` для
full/prorated — план берётся из состояния; при желании `plan` можно указать, он валидируется).

## Табличные тесты (rule_grace_test.go, пишет mechanic)

Через публичный `Compute` (каталог: `pro-month` = 2000, month, USD):

1. **payment_failed без подписки** — один `payment_failed pro-month` (2026-01-01) → ошибка
   `prorata: grace: no active subscription`;
2. **payment_failed на триале** — `trial_start pro-month` (2026-01-01) + `payment_failed
   pro-month` (2026-01-05) → ошибка `prorata: grace: cannot fail payment on a trial`;
3. **двойной payment_failed** — `subscribe pro-month` (2026-01-01) + `payment_failed pro-month`
   (2026-01-05) + `payment_failed pro-month` (2026-01-08) → ошибка
   `prorata: grace: already in grace`;
4. **grace_recover без грейса** — `subscribe pro-month` (2026-01-01) + `grace_recover pro-month`
   (2026-01-05) → ошибка `prorata: grace: no grace in progress`;
5. **grace_expire без грейса (пустое состояние)** — один `grace_expire pro-month` (2026-01-01)
   → ошибка `prorata: grace: no grace in progress`;
6. **неизвестный план** — `subscribe pro-month` (2026-01-01) + `payment_failed ghost`
   (2026-01-05) → ошибка `prorata: grace: unknown plan "ghost"`.

## Gate

_(пусто — заполняется architect'ом на гейт-ревью диффа `rule_grace.go`)_
