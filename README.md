# Open-Kaurava

База для игры/движка: окно ОС + Vulkan **без CGO** на Windows и Linux из одного
кода. Загрузка библиотек и вызовы нативных функций идут через
[purego](https://github.com/ebitengine/purego) — условная компиляция нужна
только там, где ОС действительно различаются: создание окна и создание
Vulkan-поверхности.

```
├── internal/dylib/       # загрузка .dll/.so (единственное место с #ifdef по ОС)
├── vulkan/               # 100% кроссплатформенные биндинги Vulkan
│   ├── api.go            #   загрузчик: vkGetInstanceProcAddr → instance → device
│   ├── structs.go        #   Go-структуры с раскладкой vulkan_core.h
│   ├── instance.go       #   создание инстанса, debug-мессенджер
│   ├── device.go         #   выбор и создание устройства
│   ├── surface.go        #   поверхности (платформенные вызовы — в window)
│   ├── swapchain.go      #   swapchain, image views, render pass, фреймбуферы
│   ├── render.go         #   командные буферы, кадровая синхронизация, Record
│   ├── debug.go          #   VK_EXT_debug_utils (валидация)
│   ├── result.go         #   VkResult → понятные ошибки
│   └── layout_test.go    #   проверка раскладок структур (+ tools/layout_check.py)
├── window/               # платформозависимый слой (окна и ввод)
│   ├── window.go         #   общий API: New, PollEvents, CreateSurface, мышь
│   ├── drag.go           #   трекер кликов/драгов (чистая логика, юнит-тесты)
│   ├── window_windows.go #   Win32 (user32 через purego)
│   ├── window_x11.go     #   X11 (libX11 через purego; работает и под XWayland/WSLg)
│   └── window_unsupported.go # заглушка для остальных ОС
├── tools/layout_check.py # независимый расчёт раскладок на ctypes
└── main.go               # рабочее демо: окно + swapchain + курсор + драг
```

## Запуск

```
go run .                 # окно с анимированной заливкой, рисованный курсор
go run . -frames 300     # отрисовать 300 кадров и выйти (удобно для проверок)
go run . -validation     # включить слои валидации, если они установлены
go run . -log-events     # печатать события окна (клавиши, мышь, resize)
go run . -os-cursor      # оставить системный курсор вместо рисуемого крестика
go run . -resize-test    # сам менять размер окна и проверять пересоздание swapchain
go run . -present fifo   # принудительный режим present'а: fifo|mailbox|immediate
```

Требования: Windows 10+ или Linux с X11/XWayland (WSLg подходит) и драйвером
Vulkan (NVIDIA/AMD/Intel или Mesa/llvmpipe). CGO не нужен: `CGO_ENABLED=0`.

## Архитектура

Три слоя, каждый знает только о соседе:

1. **`vulkan`** — биндинги, полностью не зависящие от ОС. Один `API`-объект
   соответствует одному `VkInstance` + одному `VkDevice`. Функции уровня
   device загружаются в `CreateDevice`, расширения — по требованию через
   `api.Proc("vk...")`.
2. **`window`** — создание окна, событийная очередь и нативные хэндлы.
   Каждый бэкенд отдаёт `SurfaceExtension()` и `CreateSurface()`, а весь вызов
   Vulkan остаётся кроссплатформенным кодом.
3. **`main.go`** — цикл кадра: `PollEvents` → логика → `Record` → submit → present.

Ключевые решения:

- **Одна точка загрузки ОС.** `internal/dylib` — единственное место, где есть
  `//go:build`: `LoadLibraryW` на Windows, `dlopen` на Unix. Всё остальное
  (purego.RegisterFunc/RegisterLibFunc) одинаково везде.
- **Раскладки структур проверяются дважды.** Ожидания в `layout_test.go`
  выведены из `vulkan_core.h` вручную; `tools/layout_check.py` пересчитывает
  те же структуры на ctypes и печатает в формате `TestLayoutPrint` — два
  вывода сравниваются диффом:

  ```
  VULKAN_LAYOUT_PRINT=1 go test ./vulkan -run TestLayoutPrint > go.txt
  python3 tools/layout_check.py > py.txt
  diff go.txt py.txt
  ```

  Плюс canary-тест `TestPhysicalDevicePropertiesFits` прогоняется против
  реального драйвера (Intel/NVIDIA на Windows, llvmpipe в WSL) и ловит
  расхождение раскладки «в живую».
- **Окно и Vulkan в одном потоке.** `runtime.LockOSThread()` в main обязателен:
  Win32 доставляет сообщения создавшему окно потоку, Xlib не потокобезопасен.
- **Кадр без шейдеров.** Базовый `Record` заливает фон и рисует прямоугольники
  через `vkCmdClearAttachments` — этого хватает для курсора и рамок выделения,
  пока нет графического пайплайна.

## Ввод: события, состояние, жесты

`window` даёт три уровня:

- **События** — `PollEvents() []Event`: `EventMouseMove/Down/Up/Scroll`,
  `EventKeyDown/Up`, `EventResize`, `EventClose`. Всё, что пришло от ОС.
- **Состояние** — `MousePos()`, `MouseDown(button)`: обновляется из событий
  автоматически, читается в любой момент кадра.
- **Жесты** — трекер `window.Drag`: отличает клик от перетаскивания по
  порогу (`Threshold`, по умолчанию 5 px) и отдаёт готовые результаты:

  ```go
  var drag window.Drag
  for _, ev := range win.PollEvents() {
      drag.Feed(ev)
  }
  if x, y := drag.Pos(); ...        // позиция курсора
  for _, c := range drag.TakeClicks() { ... }   // клики за кадр
  for _, d := range drag.TakeDrags() { ... }    // завершённые драги
  if x, y, w, h, ok := drag.Rect(); ok { ... }  // рамка текущего выделения
  ```

Два важных нативных момента, скрытых за общим API:

- **Захват курсора** (`CaptureMouse(true)`) — без него драг обрывается у края
  окна: Win32 перестаёт слать сообщения, X11 под захватом продолжает.
  Демо захватывает курсор автоматически при начале драга.
- **Скрытие системного курсора** (`SetCursorVisible(false)`) — на Win32 это
  обработка `WM_SETCURSOR`, на X11 — невидимый курсор через
  `XCreatePixmapCursor` с пустой маской. Демо рисует свой крестик поверх сцены.

## Рецепты расширения

**Новый тип события.** Добавить `EventKind` в `window.go`, заполнить поля
`Event`, пушить из нужного бэкенда (`windowProcedure` на Win32, `handle` на
X11). Если событие мыши — состояние подхватит `push` автоматически.

**Новая нативная функция.** Зарегистрировать в `init()` бэкенда через
`mustReg`/`purego.RegisterFunc` — сигнатура берётся из заголовка ОС
(x64: `HWND/HINSTANCE/long` → `uintptr`, `int` → `int32`).

**Новая структура Vulkan.** Описать в `structs.go` (раскладка как в
`vulkan_core.h`), добавить в `types()` и `layoutCases()` в `layout_test.go`,
продублировать на ctypes в `tools/layout_check.py` и сверить диффом.

**Новая функция Vulkan.** Поле в `api.go`, загрузка в `loadDevice()`
(обязательная — `must`) или по требованию (`try`), вызов — метод `*API`.

**Шейдеры и графический пайплайн.** Следующий шаг эволюции базы: загрузка
SPIR-V через `vkCreateShaderModule` (уже есть `api.Proc`), пайплайн через
`vkCreateGraphicsPipelines`. `Record` расширяется записью draw-команд между
`vkCmdBeginRenderPass` и `vkCmdEndRenderPass`; прямоугольники интерфейса
переезжают в atlas-спрайты.

**Wayland.** Бэкенд `window_wayland.go` по образцу x11: `libwayland-client`,
`xdg_surface`/`xdg_toplevel`, поверхность — `VK_KHR_wayland_surface`
(константа `ExtWaylandSurface` и структура `WaylandSurfaceCreateInfoKHR`
уже ждут в пакете `vulkan`).

## Проверено

- Windows 10+ (Intel HD 620, Vulkan 1.3): сборка, тесты раскладок, canary
  против реального драйвера, демо — клик/драг/рамка выделения, скриншот.
- Linux/WSLg (XWayland + llvmpipe, Vulkan 1.3): то же самое, плюс resize-test
  (3 пересоздания swapchain за прогон).
- `go vet`, `go test ./...` — чисто; `CGO_ENABLED=0` на обеих ОС.
