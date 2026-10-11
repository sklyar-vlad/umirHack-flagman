BEGIN TRANSACTION READ ONLY;
SELECT version_id, is_applied FROM
 (SELECT DISTINCT ON (version_id) version_id,is_applied FROM goose_db_version
  ORDER BY version_id,id DESC) v ORDER BY version_id;
-- Parse and execute the reporting views, not just their names.
SELECT count(*) FROM monitoring_stock;
SELECT count(*) FROM monitoring_problem_batches;
SELECT count(*) FROM monitoring_stop_list;
SELECT count(*) FROM monitoring_losses;
-- Display counts for reconciliation with the source backup snapshot.
SELECT format('SELECT %L AS relation, count(*) AS rows FROM %I.%I;',
 table_name,table_schema,table_name)
FROM information_schema.tables
WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name
\gexec
COMMIT;
