# `gamescreen.screen` DoWDE: первый проход для HUD

Исследована штатная зависимость DoWDE, а не пользовательский UA_DE. `Data\\art\\ui\\screens\\gamescreen.screen` разрешается из `DXP3Data.sga` по зависимостям `DoWDE.module`; похожие копии есть в DXP2, W40k и Engine. Движок загружает файл из объединённого дерева модуля, поэтому более приоритетное переопределение появится в том же виртуальном пути.

## Структура экрана

`Screen` — data-only Lua chunk с `version=4`, `AspectRatio=1.33333`, `TooltipWidgets` и `Widgets`. В `Widgets` находится runtime tree с корнем `grp_Screen`, вложенными группами, named widgets, `Presentation.Art`/`Presentation.Text`. В конце файла также присутствует отдельный плоский справочник имён для редактора; layout и видимость должны идти только по иерархии `Widgets`.

В DoWDE координаты нормализованы от полного viewport, а не от геометрии родителя. Позиции предков суммируются; размеры каждого виджета считаются отдельно от viewport. Так группа `grpBackground` с отрицательным offset задаёт виртуальную широкую область, а нижний taskbar может уходить за края. Rect намеренно сохраняет отрицательные и >1 координаты; clip/crop будет следующим шагом.

| Группа | Геометрия / initial visible | Назначение и стадия |
|---|---|---|
| `grp_Screen` → `grpBackground` | корень; background `(-0.375,0)`, `(1.75,1)` | экран и виртуальное поле координат |
| `grpResourceInfo` | `(0,-0.0011)`, `(0.37608,0.14967)` | ресурсы и лимиты, включая race-specific req/power/faith |
| `grpMinimap` | `(0,0.64015)`, `(0.27664,0.36245)` | левый нижний угол; содержит `ctmMinimap` Custom и texture `taskbar_minimap.tga` |
| `grpTaskbar` | `(0,0.65)`, `(1.75,0.35)` | нижняя панель; `taskbar.lua` скрывает/показывает её binding-ом |
| `grpTaskbarLeft` | `(0.375,-0.65)`, `(1,1)` в taskbar | левый taskbar, ресурсы/мини-карта/общие кнопки |
| `grpTaskbarRight` | `(0.375,-0.65)`, `(1,1)` в taskbar | правый taskbar, команды, производство, selection widgets |
| `grpSingleSquadSelection` | `(0.28160,0.815)`, `(0.34389,0.18)`, начально скрыта | одиночный отряд, stats и portrait |
| `grpStructureSelection` | `(0.28147,0.815)`, `(0.34402,0.18)` | альтернативное состояние выбора здания |
| `grpMultiSquadSelection` | `(0.38720,0.813)`, `(0.22763,0.18)`, начально скрыта | множественное выделение |
| `grpMenuButtons` | `(0.72009,-0.00006)`, `(0.27991,0.03233)` | `btnMENU`, `btnOBJ`, `btnCHAT`, `btnDIP` |
| `grpStrategicUI` | `(0.34402,0.02237)`, `(0.65696,0.19967)` | дочерние `grpPoints`, `grpObjectives`, `grpRelics` |

## Runtime связи

- `Data\\game\\taskbar.lua` определяет bindings и aggregate tables для selection states; его комментарий связывает выбор aggregate с native `ModTaskbar::RefreshTaskbar()`. `toggle_taskbar` задаёт `taskbar_ui="grpTaskbar"`, help text и simvis. Мы пока сохраняем данные, но не исполняем эти native bindings.
- Visibility в `.screen` — authored начальное состояние: `visible=false`, а в справочном хвосте встречается legacy `hidden=true`. Скрытое состояние наследуется от предков; runtime taskbar bindings переключают selection groups позже. В прототипе показ unit/building selection placeholder выбирается по типу выбранной сущности.
- `Data\\scar\\ui.scar` реализует сценарные API для objectives, minimap pings и countdown warnings. Это отдельные script/native действия, не источник геометрии `.screen`.

## Текущий интерпретатор

`screenui` безопасно исполняет только data chunk без стандартных Lua libraries, сохраняет неизвестные свойства, разбирает hierarchy/presentation, типы, текстуры, visibility и screen-relative layout. Gameplay загружает `gamescreen.screen` из дерева штатного модуля DoWDE и интерпретирует первый набор областей по их именам и координатам: `artMenubar*`, `grpResourceInfo`, `grpMenuButtons`/`btnOBJ`/`btnDIP`/`btnMENU`, `artTaskbar*`, `grpMinimap`, selection-шаблоны и группы `grpCommandIcons`, `grpBuildQueue`, `grpUpgrades`. Контейнеры taskbar не считаются видимыми панелями: заглушки опираются на реальные `ArtLabel`/функциональные группы. Кнопки меню кликабельны; выбор юнита/постройки включает соответствующую selection-группу, а вражеская сущность не получает приказов. Геометрия mini-map, HUD и menu controls берётся из screen-узлов; размер отсутствующий у `btnDIP` выводится из соседней кнопки.

Пока намеренно остаются placeholders: отрисовка Relic `Presentation.Art`/TGA из SGA, встроенная Custom minimap, `$ID` localization, значения ресурсов, HP/morale/lifetime, реальные attack/defense modifiers, очередь производства/пополнение/rally/garrison и runtime логика SCAR/native bindings. Исходные данные этих узлов сохранены в `Properties` и дереве для последующей интерпретации; placeholder не выдаётся за значение игры.
