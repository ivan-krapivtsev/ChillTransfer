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
| T-005 | done | UI: выбор устройства, drag&drop, очередь, прогресс, настройки режимов | feature/ui-main-window ✓ в develop |
| T-006 | done | Профили устройств (персистентность настроек per-device) | feature/device-profiles ✓ в develop |
| T-007 | done | Локализация RU/EN + тёмная/светлая тема | feature/i18n-theming ✓ в develop |

## v1 — выпущена как **v0.1.0** (тег на main, 2026-10-17)
Все задачи T-001…T-007 в develop; CI зелёный на Windows/Linux/macOS.
Следующий этап: v1.x — SHA-256 верификация, resume прерванных копий, HID/IOKit датчики (Win/macOS), установщики.
