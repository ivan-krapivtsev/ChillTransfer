# Архитектура ChillTransfer

## Стек (согласовано с владельцем)
- **Язык:** Go 1.22+
- **GUI:** Fyne v2 (drag&drop файлов через `Window.SetOnDropped`, нативные диалоги, темы light/dark)
- **Платформенный слой:** Linux — stdlib (`/sys/class/hwmon`); Windows — P/Invoke (`golang.org/x/sys/windows`, SetupAPI/HID API); macOS — IOKit (cgo-ObjC только для таргета darwin; решение уточняется в T-004)
- **Пакетирование v1:** portable-бинарники (один файл на платформу, без установщика)

## Структура кода
```
chilltransfer/
├── cmd/chilltransfer/main.go        — точка входа, инициализация Fyne-приложения
├── internal/
│   ├── engine/                      — ДВИЖОК: чистая логика, без UI и платформенных зависимостей
│   │   ├── state.go                 — машина состояний (idle/copying/paused_thermal/resting/done/error/cancelled)
│   │   ├── queue.go                 — очередь элементов (файлы/папки), статусы
│   │   ├── copier.go                — рекурсивное копирование, прогресс, скорость, ETA, fsync
│   │   └── governor.go              — интерфейс Governor + SensorGovernor (гистерезис) и CycleGovernor (таймеры)
│   ├── devices/                     — ПЛАТФОРМЕННЫЙ СЛОЙ: перечисление устройств + датчики температуры
│   │   ├── devices.go               — интерфейсы Device, TemperatureSource; реестр
│   │   ├── linux.go / windows.go / darwin.go  — реализации с build tags
│   ├── profiles/                    — профили устройств (JSON в ~/.config/chilltransfer/profiles.json)
│   └── ui/                          — Fyne: окно, выбор устройства, drop-зона, очередь, настройки режимов
├── docs/                            — spec / architecture / gitflow / tasks
└── .github/workflows/ci.yml         — build matrix (ubuntu/macos/windows): vet + test + build
```

## Ключевые решения
1. **Изоляция движка.** `engine` не импортирует UI и платформенный код: только интерфейсы (`TemperatureSource`, `Clock`). Это даёт 100% unit-тестов без эмуляторов и делает код простым для верификации.
2. **Governor как стратегия.** Движок на каждом тике спрашивает `Governor.AllowCopy() bool`:
   - `SensorGovernor`: temp ≥ T_pause → пауза; temp ≤ T_resume → возобновление (гистерезис против дребезга).
   - `CycleGovernor`: W минут работы → R минут отдыха, циклически.
   Выбор режима: авто (датчик найден → sensor, иначе cycle) + ручной переключатель в UI.
3. **Датчики — best-effort.** `Device.TempSource` может быть nil (нет датчика). Обнаружение никогда не блокирует запуск; UI явно показывает активный режим и причину.
4. **Безопасность записи:** fsync после каждого файла; предупреждение при отключении устройства, к которому идёт запись.
5. **UI ↔ движок:** движок работает в отдельной goroutine; UI опрашивает снимок состояния (`StateSnapshot`) по тикеру (~2 Гц) — без блокировок и data race (snapshot под atomic/мутексом).

## Стратегия тестирования
| Слой | Метод |
|---|---|
| `engine` | Плотные unit-тесты: реальные файлы в `t.TempDir()`, fake clock, скриптуемый TemperatureSource. Покрытие governor'ов — табличные тесты (гистерезис, циклы). |
| `devices` | Тесты интерфейсов с фейками; интеграционные smoke на целевой ОС (best-effort, не блокируют CI других платформ). |
| `ui` | Headless-тесты Fyne (`fyne/test`) для ключевых сценариев + ручной smoke. |

## Цикл данных
```
UI (Fyne) ──drop/кнопки──▶ engine.Queue
engine.Copier ◀──tick── Governor ◀── TemperatureSource (devices/*)
Copier ──StateSnapshot──▶ UI (прогресс, скорость, температура, состояние)
profiles ◀──сохранение──▶ настройки режимов per-device
```
