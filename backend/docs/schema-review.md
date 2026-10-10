# S03-T01: сверка миграций с исходной DBML

## Эталон и текущий результат

Эталон — [полная схема БД](../../docs/database-schema.md), восстановленная из страницы YouGile `UMI-9`. Сравнивать нужно DBML и дополнения к SQL под ней: частичные индексы и межтабличные правила не выражены полностью внутри DBML.

На момент первой сверки `backend/migrations/` пустая. Исходная схема содержит **17 таблиц, 126 колонок, 5 enum и 20 внешних ключей**. В миграциях они пока отсутствуют. S03-T01 остаётся открытой до переноса схемы в SQL и повторной сверки. Получение эталона само по себе не означает, что БД создана.

## Таблицы

| Таблица | Колонок в DBML | На что обратить внимание |
|---|---|---|
| `product_categories` | 5 | revision и timestamps |
| `products` | 11 | category_id, оба enum единиц, nullable срок и numeric(14,3) target_stock |
| `supplies` | 5 | уникальный строковый номер, received_at отдельно от created_at, обязательный автор |
| `batches` | 11 | отдельная партия на строку, line_number, numeric(14,3), nullable produced_at/shelf_life_hours |
| `dishes` | 7 | nullable description, активность, revision |
| `recipe_versions` | 5 | версия принадлежит блюду, автор обязателен, UNIQUE(id,dish_id) для составного FK |
| `recipe_items` | 4 | ссылка на версию, не прямо на блюдо; quantity_per_portion numeric(14,3) |
| `orders` | 4 | bigint number с генерацией identity/sequence; только успешные заказы |
| `order_items` | 6 | dish_name_snapshot, конкретная версия рецепта и составной FK |
| `stock_movements` | 11 | знаковая delta_quantity, исторические имя/единица/остаток; nullable actor_id/order_item_id/note |
| `order_attempts` | 7 | JSONB снимок корзины/дефицитов/причин; это не строки orders |
| `users` | 10 | password_hash, фиксированная роль, nullable last_login_at |
| `sessions` | 7 | хеш токена отдельно от CSRF, nullable revoked_at |
| `restaurant_settings` | 7 | singleton id=1; timezone, порог, автосписание и revision |
| `audit_log` | 9 | nullable actor/entity/request/before/after; entity_id полиморфный, без общего FK |
| `idempotency_keys` | 10 | пользователь/endpoint/key, request_hash, сохранённые status/body/headers и срок |
| `outbox_events` | 7 | nullable cursor до публикации; actor_id/published_at также nullable |

Названия, SQL-типы, NOT NULL и DEFAULT каждой колонки сверять непосредственно с DBML. Сокращённые ER-диаграммы README не перечисляют все колонки.

## Enum

| Enum | Значения в исходном порядке |
|---|---|
| `product_unit` | g, ml, pcs |
| `display_unit` | g, kg, ml, l, pcs |
| `expiry_source` | explicit, calculated |
| `movement_type` | receipt, order_consumption, expiration, manual_writeoff |
| `user_role` | admin, manager, storekeeper, order_operator |

Не заменять роли или единицы свободным text и не добавлять значения из демонстрационного дизайна. Новые значения потребуют отдельного решения и миграции.

## Внешние ключи

Все 20 связей исходной DBML используют `ON DELETE RESTRICT`:

1. products.category_id → product_categories.id.
2. supplies.created_by → users.id.
3. batches.supply_id → supplies.id.
4. batches.product_id → products.id.
5. recipe_versions.dish_id → dishes.id.
6. recipe_versions.created_by → users.id.
7. recipe_items.recipe_version_id → recipe_versions.id.
8. recipe_items.product_id → products.id.
9. orders.created_by → users.id.
10. order_items.order_id → orders.id.
11. order_items.dish_id → dishes.id.
12. order_items.(recipe_version_id,dish_id) → recipe_versions.(id,dish_id).
13. stock_movements.batch_id → batches.id.
14. stock_movements.order_item_id → order_items.id.
15. stock_movements.actor_id → users.id.
16. order_attempts.created_by → users.id.
17. sessions.user_id → users.id.
18. audit_log.actor_id → users.id.
19. idempotency_keys.user_id → users.id.
20. outbox_events.actor_id → users.id.

Составная связь №12 не заменяется одиночной ссылкой recipe_version_id → recipe_versions.id: пара защищает от рецепта чужого блюда. На её целевой паре нужен UNIQUE(id,dish_id). Nullable FK допускает отсутствие автора у системного действия, но проверяет существование пользователя, когда UUID указан.

## Порядок создания таблиц

Если FK объявляются сразу внутри CREATE TABLE, подходит следующий порядок:

1. Пять enum.
2. users, product_categories.
3. products, supplies, dishes.
4. batches, recipe_versions.
5. recipe_items, orders.
6. order_items.
7. stock_movements, order_attempts, sessions, restaurant_settings, audit_log, idempotency_keys, outbox_events.

Другой вариант — сначала создать все таблицы, затем добавить FK через ALTER TABLE. В этом случае порядок таблиц не обязан совпадать с порядком зависимостей.

## Что относится к соседним задачам

- **S03-T02:** версионирование/применение SQL, журнал миграций и task migrate. Не запускать миграционный SQL как обычный запрос при каждом HTTP-старте.
- **S03-T03:** полная сверка UNIQUE/CHECK/индексов, частичный FEFO, единственное поступление партии и неопубликованный outbox. PK и UNIQUE целевой пары составного FK нужны уже для корректного создания связей S03-T01.
- **S03-T04:** операции repository и передача одной транзакции.
- **S03-T05:** общие операции записи audit/outbox в эту транзакцию.

S03-T01 не требует реализовать сервис заказов, worker или SSE. Их таблицы сохраняются в схеме заранее, чтобы следующие стори опирались на согласованную структуру.

## Приёмка S03-T01 после написания SQL

- [ ] Есть все 17 таблиц, без замены recipe_versions текущим recipe в dishes.
- [ ] Все 126 колонок соответствуют DBML по типам, nullable и default; количества numeric(14,3), моменты времени timestamptz.
- [ ] Есть все пять enum с исходными значениями.
- [ ] Есть все 20 FK, включая составную связь рецепта и блюда, с ON DELETE RESTRICT.
- [ ] Сохранены order_attempts, idempotency_keys и outbox_events со всеми колонками.
- [ ] Не добавлены дублирующие supply_items/inventory/stop_list или редактируемые таблицы ролей.
- [ ] В SQL сохранены генерация номера orders и отдельное назначение cursor outbox после предметного commit; cursor не имеет DEFAULT nextval при вставке предметного события.
- [ ] После добавления миграций выполнена повторная сверка; применение на чистой БД подтверждается вместе с S03-T02.
