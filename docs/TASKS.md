# Задачи ChillTransfer

Формат: `ID | Статус | Описание | Критерии приёмки | Ветка`.
Статусы: `planned → in_progress → testing → review → done`.

## Бэклог v1 (заполняется после согласования стека)
| ID | Статус | Задача | Ветка |
|---|---|---|---|
| T-001 | planned | Каркас проекта: workspace, CI build matrix, smoke-тест UI | feature/project-scaffold |
| T-002 | done | Ядро: очередь копирования + движок (рекурсивно, прогресс, pause/resume/cancel) | feature/copy-engine ✓ в develop |
| T-003 | done | Термический контроллер: сенсорный режим (гистерезис) + циклический режим | feature/thermal-governor ✓ в develop |
| T-004 | done | Платформенный слой: перечисление устройств + чтение температуры (Win/macOS/Linux) | feature/platform-devices ✓ в develop |
| T-005 | planned | UI: выбор устройства, drag&drop, очередь, прогресс, настройки режимов | feature/ui-main-window |
| T-006 | planned | Профили устройств (персистентность настроек per-device) | feature/device-profiles |
| T-007 | planned | Локализация RU/EN + тёмная/светлая тема | feature/i18n-theming |
