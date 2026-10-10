# Кладовая — полная схема БД

Источник: [страница «схема БД» в YouGile, UMI-9](https://ru.yougile.com/team/0608bf797048/umirHack__dash__flagman/doc/UMI-9). Копия исходной схемы для S03-T01; SQL-миграции сверяются с DBML и дополнениями ниже.

Это проектная схема PostgreSQL 18 для контрактов [README](../README.md). Она включает основной складской учёт, админку из руководства дизайна и служебные таблицы для идемпотентности/SSE. Миграции ещё не реализованы. Поле не становится реализованным только из-за присутствия в диаграмме.

## **Таблицы и связи**

| Таблица | Назначение и связи |
| --- | --- |
| `product_categories` | Категория → несколько продуктов |
| `products` | Справочник: базовая/отображаемая единица, стандартный срок, целевой запас |
| `supplies` | Факт приёмки, уникальный номер, даты, автор → несколько партий |
| `batches` | Строка поставки и партия продукта, исходное/оставшееся количество и дедлайн |
| `dishes` | Справочник блюд → неизменяемые версии рецептов |
| `recipe_versions` | Версия состава одного блюда → строки рецепта |
| `recipe_items` | Продукт и расход на одну порцию конкретной версии |
| `orders` | Только успешно оформленные заказы, номер, дата, автор |
| `order_items` | Блюдо, зафиксированная версия рецепта, имя на момент заказа, порции |
| `stock_movements` | Поступление/списание конкретной партии; потребление связано с позицией заказа |
| `order_attempts` | Отклонённые оформления со снимком корзины и дефицитов, без заказа и движения |
| `users` | Пользователи, четыре фиксированные роли, активность, пароль-хеш |
| `sessions` | Cookie-сессии: хеш токена, CSRF, срок/отзыв |
| `restaurant_settings` | Одна строка настроек одного ресторана и склада |
| `audit_log` | Кто/что изменил, разрешённые снимки до/после; без секретов |
| `idempotency_keys` | Область пользователя/эндпоинта/ключа и сохранённый HTTP-результат |
| `outbox_events` | События изменений; cursor SSE назначается после предметного commit |

Остатки продукта, статусы партий, стоп-лист, рекомендации и отчёты вычисляются. Промежуточная таблица `supply_items` не нужна: каждая строка приёмки уже является записью `batches`. Отдельные таблицы `stop_list`/`hot_offers` создали бы дублирующую информацию, зависящую от текущего времени.

## **DBML для dbdiagram.io**

Скопируйте блок целиком в dbdiagram.io. Колонки, составные связи, выражения индексов и CHECK записаны согласно [синтаксису DBML](https://dbml.dbdiagram.io/docs/). Частичные индексы PostgreSQL и межтабличные правила перечислены после блока: они требуют дополнения SQL-миграций.

```dbml
Project kladovaya {
  database_type: 'PostgreSQL'
  Note: 'One restaurant, one warehouse. All stock quantities use base units.'
}

Enum product_unit {
  g
  ml
  pcs
}

Enum display_unit {
  g
  kg
  ml
  l
  pcs
}

Enum expiry_source {
  explicit
  calculated
}

Enum movement_type {
  receipt
  order_consumption
  expiration
  manual_writeoff
}

Enum user_role {
  admin
  manager
  storekeeper
  order_operator
}

Table product_categories {
  id uuid [pk, default: `gen_random_uuid()`]
  name varchar(200) [not null]
  revision int [not null, default: 1]
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]

  indexes {
    (`lower(name)`) [unique, name: 'categories_name_ci_uidx']
  }
  checks {
    `length(btrim(name)) > 0` [name: 'categories_name_nonempty']
    `revision > 0` [name: 'categories_revision_positive']
  }
}

Table products {
  id uuid [pk, default: `gen_random_uuid()`]
  category_id uuid [not null]
  name varchar(200) [not null]
  unit product_unit [not null]
  display_unit display_unit [not null]
  standard_shelf_life_hours int
  target_stock numeric(14,3)
  is_active boolean [not null, default: true]
  revision int [not null, default: 1]
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]

  indexes {
    (`lower(name)`) [unique, name: 'products_name_ci_uidx']
    category_id
  }
  checks {
    `length(btrim(name)) > 0` [name: 'products_name_nonempty']
    `standard_shelf_life_hours IS NULL OR standard_shelf_life_hours > 0` [name: 'products_shelf_life_positive']
    `target_stock IS NULL OR target_stock >= 0` [name: 'products_target_nonnegative']
    `unit <> 'pcs' OR target_stock IS NULL OR target_stock = trunc(target_stock)` [name: 'products_target_pcs_integer']
    `(unit = 'g' AND display_unit IN ('g','kg')) OR (unit = 'ml' AND display_unit IN ('ml','l')) OR (unit = 'pcs' AND display_unit = 'pcs')` [name: 'products_display_unit_compatible']
    `revision > 0` [name: 'products_revision_positive']
  }
}

Table supplies {
  id uuid [pk, default: `gen_random_uuid()`]
  number varchar(80) [not null, unique]
  received_at timestamptz [not null]
  created_at timestamptz [not null, default: `now()`]
  created_by uuid [not null]

  indexes {
    (created_at, id)
  }
  checks {
    `length(btrim(number)) > 0 AND number = btrim(number)` [name: 'supplies_number_trimmed']
  }
}

Table batches {
  id uuid [pk, default: `gen_random_uuid()`]
  supply_id uuid [not null]
  product_id uuid [not null]
  line_number int [not null]
  initial_quantity numeric(14,3) [not null]
  remaining_quantity numeric(14,3) [not null]
  produced_at timestamptz
  shelf_life_hours int
  expiry_source expiry_source [not null]
  expires_at timestamptz [not null]
  created_at timestamptz [not null, default: `now()`]

  indexes {
    (supply_id, line_number) [unique]
    (product_id, expires_at, id) [name: 'batches_fefo_base_idx']
  }
  checks {
    `line_number > 0` [name: 'batches_line_positive']
    `initial_quantity > 0` [name: 'batches_initial_positive']
    `remaining_quantity >= 0 AND remaining_quantity <= initial_quantity` [name: 'batches_remaining_range']
    `produced_at IS NULL OR expires_at > produced_at` [name: 'batches_expiry_after_production']
    `(expiry_source = 'explicit' AND shelf_life_hours IS NULL) OR (expiry_source = 'calculated' AND produced_at IS NOT NULL AND shelf_life_hours IS NOT NULL AND shelf_life_hours > 0)` [name: 'batches_expiry_input_consistent']
  }
}

Table dishes {
  id uuid [pk, default: `gen_random_uuid()`]
  name varchar(200) [not null]
  description varchar(2000)
  is_active boolean [not null, default: true]
  revision int [not null, default: 1]
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]

  indexes {
    (`lower(name)`) [unique, name: 'dishes_name_ci_uidx']
  }
  checks {
    `length(btrim(name)) > 0` [name: 'dishes_name_nonempty']
    `revision > 0` [name: 'dishes_revision_positive']
  }
}

Table recipe_versions {
  id uuid [pk, default: `gen_random_uuid()`]
  dish_id uuid [not null]
  version int [not null]
  created_at timestamptz [not null, default: `now()`]
  created_by uuid [not null]

  indexes {
    (dish_id, version) [unique]
    (id, dish_id) [unique, note: 'Target of composite FK from order_items']
  }
  checks {
    `version > 0` [name: 'recipe_version_positive']
  }
}

Table recipe_items {
  id uuid [pk, default: `gen_random_uuid()`]
  recipe_version_id uuid [not null]
  product_id uuid [not null]
  quantity_per_portion numeric(14,3) [not null]

  indexes {
    (recipe_version_id, product_id) [unique]
    product_id
  }
  checks {
    `quantity_per_portion > 0` [name: 'recipe_quantity_positive']
  }
}

Table orders {
  id uuid [pk, default: `gen_random_uuid()`]
  number bigint [not null, unique, increment]
  created_at timestamptz [not null, default: `now()`]
  created_by uuid [not null]
  Note: 'Only completed orders; API status is always completed. Rejected attempts are separate.'

  indexes {
    (created_at, id)
  }
}

Table order_items {
  id uuid [pk, default: `gen_random_uuid()`]
  order_id uuid [not null]
  dish_id uuid [not null]
  recipe_version_id uuid [not null]
  dish_name_snapshot varchar(200) [not null]
  portions int [not null]

  indexes {
    (order_id, dish_id) [unique]
    (recipe_version_id, dish_id)
  }
  checks {
    `portions BETWEEN 1 AND 1000` [name: 'order_portions_range']
  }
}

Table stock_movements {
  id uuid [pk, default: `gen_random_uuid()`]
  batch_id uuid [not null]
  order_item_id uuid
  type movement_type [not null]
  delta_quantity numeric(14,3) [not null]
  remaining_quantity_after numeric(14,3) [not null]
  product_name_snapshot varchar(200) [not null]
  unit_snapshot product_unit [not null]
  actor_id uuid
  occurred_at timestamptz [not null]
  note varchar(500)

  indexes {
    (batch_id, occurred_at, id)
    order_item_id
    (type, occurred_at)
  }
  checks {
    `remaining_quantity_after >= 0` [name: 'movement_remaining_nonnegative']
    `(type = 'receipt' AND delta_quantity > 0) OR (type <> 'receipt' AND delta_quantity < 0)` [name: 'movement_sign']
    `(type = 'order_consumption' AND order_item_id IS NOT NULL) OR (type <> 'order_consumption' AND order_item_id IS NULL)` [name: 'movement_order_link']
    `type <> 'manual_writeoff' OR (note IS NOT NULL AND length(btrim(note)) > 0)` [name: 'movement_manual_reason']
  }
}

Table order_attempts {
  id uuid [pk, default: `gen_random_uuid()`]
  created_by uuid [not null]
  created_at timestamptz [not null, default: `now()`]
  reason_code varchar(40) [not null]
  items_snapshot jsonb [not null, note: 'Normalized basket, dish names, portions, recipe versions']
  shortages jsonb [not null, default: `'[]'::jsonb`]
  issues jsonb [not null, default: `'[]'::jsonb`]
  Note: 'Only final business rejections from POST orders; no previews or transport errors.'

  indexes {
    (created_at, id)
  }
}

Table users {
  id uuid [pk, default: `gen_random_uuid()`]
  name varchar(200) [not null]
  email varchar(254) [not null, unique]
  password_hash text [not null]
  role user_role [not null]
  is_active boolean [not null, default: true]
  revision int [not null, default: 1]
  created_at timestamptz [not null, default: `now()`]
  updated_at timestamptz [not null, default: `now()`]
  last_login_at timestamptz

  checks {
    `email = lower(btrim(email))` [name: 'users_email_normalized']
    `length(btrim(name)) > 0` [name: 'users_name_nonempty']
    `revision > 0` [name: 'users_revision_positive']
  }
}

Table sessions {
  id uuid [pk, default: `gen_random_uuid()`]
  user_id uuid [not null]
  token_hash varchar(128) [not null, unique]
  csrf_token varchar(128) [not null]
  created_at timestamptz [not null, default: `now()`]
  expires_at timestamptz [not null]
  revoked_at timestamptz

  indexes {
    user_id
    expires_at
  }
  checks {
    `expires_at > created_at` [name: 'session_expiry_future']
  }
}

Table restaurant_settings {
  id int [pk, default: 1]
  restaurant_name varchar(200) [not null]
  timezone varchar(100) [not null, default: 'Europe/Moscow']
  expiring_threshold_hours int [not null, default: 24]
  auto_writeoff_expired boolean [not null, default: false]
  revision int [not null, default: 1]
  updated_at timestamptz [not null, default: `now()`]

  checks {
    `id = 1` [name: 'settings_singleton']
    `length(btrim(restaurant_name)) > 0` [name: 'settings_name_nonempty']
    `expiring_threshold_hours BETWEEN 1 AND 720` [name: 'settings_threshold_range']
    `revision > 0` [name: 'settings_revision_positive']
  }
}

Table audit_log {
  id uuid [pk, default: `gen_random_uuid()`]
  actor_id uuid
  action varchar(100) [not null]
  entity_type varchar(50) [not null]
  entity_id uuid [note: 'Nullable for singleton settings; polymorphic object, not a generic FK']
  request_id varchar(128)
  before_data jsonb
  after_data jsonb
  created_at timestamptz [not null]

  indexes {
    (created_at, id)
    (entity_type, entity_id, created_at)
    (actor_id, created_at)
  }
}

Table idempotency_keys {
  id uuid [pk, default: `gen_random_uuid()`]
  user_id uuid [not null]
  endpoint varchar(200) [not null, note: 'HTTP method and canonical path']
  key uuid [not null]
  request_hash varchar(128) [not null]
  response_status int [not null]
  response_body jsonb [not null]
  response_headers jsonb [not null, default: `'{}'::jsonb`, note: 'Only reproducible headers such as Location']
  created_at timestamptz [not null]
  expires_at timestamptz [not null]
  Note: 'Completed results. Serialize concurrent requests by scoped advisory lock before looking up/inserting.'

  indexes {
    (user_id, endpoint, key) [unique]
    expires_at
  }
  checks {
    `response_status BETWEEN 200 AND 499` [name: 'idempotency_result_status']
    `expires_at > created_at` [name: 'idempotency_expiry_future']
  }
}

Table outbox_events {
  id uuid [pk, default: `gen_random_uuid()`]
  cursor bigint [unique, note: 'Nullable before dispatcher publication; sequence assigned after subject transaction commits']
  event_type varchar(100) [not null]
  actor_id uuid
  payload jsonb [not null]
  created_at timestamptz [not null]
  published_at timestamptz

  indexes {
    (published_at, created_at, id)
  }
  checks {
    `(cursor IS NULL AND published_at IS NULL) OR (cursor IS NOT NULL AND cursor > 0 AND published_at IS NOT NULL)` [name: 'outbox_publication_consistent']
  }
}

Ref: products.category_id > product_categories.id [delete: restrict]
Ref: supplies.created_by > users.id [delete: restrict]
Ref: batches.supply_id > supplies.id [delete: restrict]
Ref: batches.product_id > products.id [delete: restrict]
Ref: recipe_versions.dish_id > dishes.id [delete: restrict]
Ref: recipe_versions.created_by > users.id [delete: restrict]
Ref: recipe_items.recipe_version_id > recipe_versions.id [delete: restrict]
Ref: recipe_items.product_id > products.id [delete: restrict]
Ref: orders.created_by > users.id [delete: restrict]
Ref: order_items.order_id > orders.id [delete: restrict]
Ref: order_items.dish_id > dishes.id [delete: restrict]
Ref: order_items.(recipe_version_id, dish_id) > recipe_versions.(id, dish_id) [delete: restrict]
Ref: stock_movements.batch_id > batches.id [delete: restrict]
Ref: stock_movements.order_item_id > order_items.id [delete: restrict]
Ref: stock_movements.actor_id > users.id [delete: restrict]
Ref: order_attempts.created_by > users.id [delete: restrict]
Ref: sessions.user_id > users.id [delete: restrict]
Ref: audit_log.actor_id > users.id [delete: restrict]
Ref: idempotency_keys.user_id > users.id [delete: restrict]
Ref: outbox_events.actor_id > users.id [delete: restrict]
```

## **Дополнения к SQL-миграциям**

DBML показывает обычный индекс FEFO. В PostgreSQL замените его частичным индексом ненулевых остатков; время годности проверяется запросом. Частичные индексы описаны в [документации PostgreSQL](https://www.postgresql.org/docs/18/indexes-partial.html).

```sql
CREATE INDEX batches_fefo_idx
    ON batches (product_id, expires_at, id)
    WHERE remaining_quantity > 0;

CREATE UNIQUE INDEX batch_receipt_once_idx
    ON stock_movements (batch_id)
    WHERE type = 'receipt';

CREATE INDEX outbox_unpublished_idx
    ON outbox_events (created_at, id)
    WHERE published_at IS NULL;
```

`orders.number` реализуется identity/sequence; SSE cursor использует отдельную sequence, которую вызывает единственный сериализованный диспетчер после commit предметных транзакций. Не используйте ID вставки outbox как cursor доставки: транзакции могут завершаться в другом порядке.

DBML допускает CHECK рассчитанной даты, но миграции должны учитывать поведение временных зон. `expires_at` записывается сервисом после прибавления реального количества часов к UTC produced\_at. Проверки `expires_at > received_at`, соответствие `pcs` в партиях/рецептах и запрет смены единицы используются под предметными блокировками в сервисе или межтабличных триггерах. Обычный CHECK не должен читать другие таблицы.

Время создания заказа/приёмки, используемое в расчётах, устанавливается сервером. `now()` в PostgreSQL связано с началом транзакции, поэтому для окончательной проверки после ожидания блокировок нужно получить свежее время (например, `clock_timestamp()`), затем использовать один зафиксированный T для всей проверки. Нельзя принимать просрочку, проверив срок только до ожидания.

## **Инварианты бизнес-операций**

1. В поставке есть хотя бы одна партия; в заказе — хотя бы одна позиция; версия рецепта содержит хотя бы один ингредиент. Проверяется на уровне транзакционного сервиса: внешний ключ не гарантирует наличие детей.
2. К каждой партии ровно одно движение поступления с `delta_quantity = initial_quantity`. Сумма всех её движений равна `remaining_quantity`.
3. `remaining_quantity_after` — состояние сразу после конкретного движения. Если из одной партии расходуются ингредиенты двух позиций заказа, состояния идут последовательно и сохраняются отдельно.
4. Потребление заказа ссылается на продукт из зафиксированной версии рецепта. Сумма потребления по продукту/позиции равна расходу на порцию, умноженному на portions. Проверяется сервисом, использующим эти же записи рецепта.
5. Названия блюд снимком хранятся в order\_items; названия продуктов и единицы для журнала — в stock\_movements. API исторических списаний использует эти снимки. IDs продолжают ссылаться на исходные справочники.
6. Свойства партии приёмки и все строки исторического рецепта неизменяемы. В партии меняется только remaining\_quantity через складскую операцию. Новая рецептура — новая версия под блокировкой блюда.
7. Отказ при нехватке не создаёт orders/order\_items/stock\_movements и не меняет партии. Отдельная запись attempts и ответ идемпотентности могут быть зафиксированы.
8. Предметная запись, аудит, ответ идемпотентности и outbox сохраняются атомарно. Включение SSE допускается после реализации его таблицы и диспетчера.
9. Область ключа идемпотентности блокируется перед чтением завершённого результата. Неполные HTTP-результаты не сохраняются в таблицу. Все записывающие операции используют единый порядок блокировок блюд → продуктов → партий; [строковые блокировки PostgreSQL](https://www.postgresql.org/docs/18/explicit-locking.html) удерживаются до конца транзакции.
10. Секреты не входят в JSON audit\_log, outbox и response\_body идемпотентности. В sessions токен cookie хранится только как хеш; csrf\_token используется отдельно от cookie.

`numeric` сохраняет точные десятичные количества согласно [документации PostgreSQL](https://www.postgresql.org/docs/18/datatype-numeric.html). В API передаются строки. Для чисел больше допустимой точности/масштаба сервер возвращает 422 до записи; округление СУБД не используется как валидация.

## **Объём реализации**

Core: product\_categories, products, supplies, batches, dishes, recipe\_versions, recipe\_items, orders, order\_items, stock\_movements. Пользователи, сессии и настройки добавляются к версии с правами; audit\_log, order\_attempts и idempotency\_keys — к соответствующим операциям. Демо с данными seed может использовать заранее созданного системного пользователя, чтобы обязательные created\_by оставались настоящими FK. Создание seed-справочников не должно обходить ограничения целостности.

Для SSE нужна outbox\_events. Аналитика списаний/дефицита и простой прогноз используют уже накопленные movements/attempts; отдельные таблицы агрегатов для MVP не нужны. Несколько ресторанов, денежные поля, резервы корзины, оплаты и отмены не входят в эту схему.
