# Набор инструментов для внешних коннекторов Scheduler на Python

## Установка и команды

```bash
python -m pip install -e .
scheduler-connector-python validate snapshot.json
scheduler-connector-python push connector.json snapshot.json
scheduler-connector-python status connector.json RUN_ID
scheduler-connector-python heartbeat connector.json
```

## Повторные попытки

SDK автоматически повторяет сетевые ошибки, HTTP `429` и `5xx` с экспоненциальной задержкой.
Повтор использует тот же `snapshot_id`/`Idempotency-Key`, но новую подпись и значение `nonce`, поэтому потерянный
ответ не создаст вторую публикацию. Параметры `max_attempts` и `base_delay` можно передать в
`ConnectorClient`.

## Закрытый ключ

Файл `connector.json` выдаётся административным мастером один раз. Не
добавляйте его в Git и не передавайте закрытый ключ третьим лицам.
