// Package entities содержит базовую модель создаваемых в игровом мире сущностей.
package entities

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// PlayerID идентифицирует игрока-владельца. Пустой ID означает, что
// владелец ещё не назначен; NeutralPlayer — явный нейтральный владелец.
type PlayerID string

const NeutralPlayer PlayerID = "neutral"

// Player — зарегистрированный игрок и список разрешённых ему типов производства.
type Player struct {
	ID                PlayerID
	Name              string
	AllowedProduction []Kind
	// Resources — стартовые ресурсы (racebps/starting_res_normal).
	Resources Resources
	// IncomeCap — потолок прироста ресурсов в секунду (racebps/income_cap);
	// нулевой — потолка нет.
	IncomeCap Income
	// SquadCapMax/SupportCapMax — с чего начинается лимит отрядов и техники
	// (racebps/race_squad_cap_table); постройки добавляют к нему свои CapBonus.
	SquadCapMax   int
	SupportCapMax int
}

// Resources — кошелёк игрока: сколько ресурсов у него сейчас есть.
type Resources struct {
	Requisition float64
	Power       float64
	Faith       float64
}

// Income — прирост ресурсов в секунду. Считается из чертежей (resource_ext)
// и ограничивается IncomeCap расы.
type Income struct {
	Requisition float64
	Power       float64
	Faith       float64
}

// CapUsage — сколько единиц лимита отрядов и техники занято.
type CapUsage struct {
	Squad   int
	Support int
}

// CapBonus — сколько единиц лимита добавляют постройки игрока.
type CapBonus struct {
	Squad   int
	Support int
}

// Economy — срез экономики игрока для панели ресурсов.
type Economy struct {
	Resources Resources
	Income    Income
	// SquadUsed/SupportUsed — занято единиц лимита; *Max — текущий потолок
	// (база расы плюс бонусы построек).
	SquadUsed   int
	SquadMax    int
	SupportUsed int
	SupportMax  int
}

// Kind определяет базовый тип сущности.
type Kind uint8

const (
	KindUnit Kind = iota + 1
	KindBuilding
	KindSummoned
	KindGameplay
)

func (k Kind) valid() bool { return k >= KindUnit && k <= KindGameplay }

// Position — положение сущности на карте в мировых координатах.
type Position struct {
	X, Y float64
}

// SelectionUI соответствует полям DATASEUI для индикатора выбранной сущности:
// DisplayType=0 — круг, 1 — квадрат; Scale — полуразмеры, Offset — смещение.
type SelectionUI struct {
	DisplayType int32
	Scale       [2]float64
	Offset      [2]float64
}

// Spec описывает новую сущность. Если Owner не задан, владельцем становится
// ProducedBy; пустые оба поля означают, что владелец будет назначен позже.
type Spec struct {
	Name             string
	Kind             Kind
	ProducedBy       PlayerID
	Owner            PlayerID
	Selectable       bool
	Capturable       bool
	Indestructible   bool   // объект нельзя уничтожить — только захватить (точки захвата).
	BuiltOn          string // ID точки захвата, на которой стоит здание; постройка забирает точку себе.
	SelectionUI      SelectionUI
	ConstructionWork float64 // > 0 задаёт незавершённое строительство здания.
	// StartHealthRatio — доля здоровья, с которой появляется стройплощадка
	// (tuning/building_info/start_health_percent). 0 — ConstructionStartHealth.
	StartHealthRatio float64
	// Income — прирост ресурсов в секунду, который даёт эта сущность.
	Income Income
	// CapUsage — сколько единиц лимита отрядов/техники занимает сущность.
	CapUsage CapUsage
	// CapBonus — лимиты, которые даёт постройка (модификаторы из чертежа).
	CapBonus CapBonus
	// Speed — скорость передвижения (moving_ext/speed_max) в единицах мира
	// в секунду; 0 — сущность не двигается (постройки).
	Speed float64
	// AccelTime, BrakeTime — секунды разгона до Speed и торможения до
	// остановки (moving_ext/acceleration_time и deceleration_time).
	// 0 — без данных: движение постоянной скоростью.
	AccelTime float64
	BrakeTime float64
	// HitPoints — здоровье по чертежу (health_ext/hitpoints); 0 — чертёж
	// не дал данных. Заполняет State.Health при спавне.
	HitPoints float64
	// MoraleMax — полная шкала морали отряда (sbps squad_morale_ext): у
	// обычных отрядов 100, у техники и построек 0 — морали нет.
	MoraleMax float64
	// Dead — сущность появляется мёртвой (труп лорда некронов на старте):
	// State.Alive остаётся ложью, здоровье обнуляется.
	Dead bool
	// UsableBody — health_ext/spawn_usable_body_on_death: тело после гибели
	// остаётся на карте и годится для сборки томб-спайдером или подъёма
	// воскрешением лорда (у некронов — воины, бессмертные, ободранные).
	UsableBody bool
	// FromBuilding — постройка, которая «пробудилась» в этого юнита (так
	// восстановленный монолит появляется из монолита-постройки): при его
	// гибели постройка возвращается. Пусто — обычный юнит.
	FromBuilding string
	// GetUpChance — шанс подняться после уничтожения (health_ext/
	// get_back_up_chance, у некронов 0.25), GetUpHealthPercent — доля
	// здоровья при подъёме, GetUpTime — сколько секунд тело лежит.
	GetUpChance        float64
	GetUpHealthPercent float64
	GetUpTime          float64
	// SquadID — отряд, в который входит боец: бойцы одного отряда
	// расталкиваются, чтобы не стоять друг в друге. Пусто — одиночный
	// объект (постройка, боец из сценария).
	SquadID string
	// Weapon — дальнобойное оружие бойца (combat_ext/hardpoints чертежа);
	// Half — полуширина и полуглубина основания в метрах мира (по X и Y):
	// по ним считается, докуда стрелку подходить к цели, чтобы его оружие
	// доставало до стены постройки, а не до середины её габаритов. Нули —
	// взять радиус по обеим осям.
	Half [2]float64
	// nil — стрелять нечем. Radius — радиус тела в метрах мира: по нему
	// считается дистанция до поверхности цели и остановка на атаке.
	Weapon *Weapon
	Radius float64
	// ArmourClass — класс брони (type_ext/type_armour), по нему оружие
	// выбирает процент пробития; Armour — health_ext/armour (у DoWDE 100).
	ArmourClass string
	Armour      float64
	// DisableAutoAttack выключает автоатаку (по умолчанию она включена:
	// боец сам стреляет по ближайшему врагу в дистанции оружия).
	DisableAutoAttack bool
}

// ConstructionStartHealth — доля здоровья, с которой появляется
// стройплощадка: остальное «достраивается» вместе с прогрессом, и к 100%
// работы здоровье возвращается к максимуму чертежа.
const ConstructionStartHealth = 0.1

// Construction хранит состояние строительства здания.
type Construction struct {
	Required float64
	Progress float64
	Builders []string // ID юнитов-строителей, назначенных фабрикой.
	// StartHealth — доля здоровья на момент закладки; от неё здоровье
	// растёт к максимуму вместе с прогрессом (см. Contribute).
	StartHealth float64
}

// Entity — сущность, зарегистрированная фабрикой. Значение, возвращаемое
// методами Factory, является снимком; изменять состояние нужно через фабрику.
//
// State — встроенное живое состояние (позиция, скорость, здоровье, мораль,
// модификаторы, приказ со статусом): его модель и движение лежат в
// state.go, здесь — только реестр, владение и экономика.
type Entity struct {
	ID         string
	Name       string
	Kind       Kind
	ProducedBy PlayerID
	Owner      PlayerID
	State
	Selectable bool
	Capturable bool
	// Indestructible — объект нельзя уничтожить (слаг-депозиты, стратегические
	// позиции и объекты, реликтовые точки): только захват.
	Indestructible bool
	// BuiltOn — точка захвата, на которой это здание построено.
	BuiltOn string // Facing — куда смотрит объект сейчас: направление фактического шага по
	// маршруту (радианы, 0 — вдоль +X), а не конечная точка приказа.
	Facing float64
	// SquadID — отряд бойца (см. Spec.SquadID): по нему идёт расталкивание.
	SquadID string
	// Weapon, Radius, ArmourClass и Armour — боевые данные бойца: чем он
	// стреляет, какой он величины и по какой броне в него попадают.
	Weapon *Weapon
	Radius float64
	// Half — полуоси основания: у построек они длиннее радиуса и разной
	// длины по осям, поэтому дистанцию до цели точнее даёт бокс, а не круг.
	Half [2]float64
	// GetUpChance, GetUpHealthPercent, GetUpTime — восстановление после
	// уничтожения (некроны): шанс, доля здоровья и задержка до подъёма.
	// ReviveIn — остаток задержки; > 0 означает лежащее тело, которое
	// встанет само.
	GetUpChance        float64
	GetUpHealthPercent float64
	GetUpTime          float64
	ReviveIn           float64
	// UsableBody — тело годится для сборки/воскрешения (см. Spec).
	UsableBody bool
	// CorpseTaken — тело уже собрано спайдером или поднято воскрешением:
	// второй раз оно не используется. При новой гибели сбрасывается.
	CorpseTaken bool
	// CaptureProgress — ход захвата самой точки (0..1), CapturePlayer — тот,
	// кто ставит на ней флаг. У захваченной точки прогресс 1, а
	// CapturePlayer совпадает с владельцем; при снятии чужого флага
	// прогресс идёт вниз вдвое быстрее (см. stepCapture).
	CaptureProgress float64
	CapturePlayer   PlayerID
	// FromBuilding — постройка, из которой пробудился юнит (см. Spec):
	// при гибели юнита она возвращается на своё место.
	FromBuilding string
	// Dormant — постройка «спит»: она занята пробудившимся юнитом, не
	// выбирается, не атакуется, ничего не производит и не даёт дохода.
	Dormant     bool
	ArmourClass string
	Armour      float64
	SelectionUI SelectionUI
	Build       Construction
	// Income, CapUsage, CapBonus — то, что сущность вносит в экономику
	// владельца: прирост ресурсов и расход/прибавка лимитов.
	Income   Income
	CapUsage CapUsage
	CapBonus CapBonus
}

// UnderConstruction сообщает, осталось ли выполнить строительную работу.
// SurfaceGap — расстояние от точки (x, y) до основания сущности в
// направлении на неё: расстояние до центра минус выход бокса основания по
// этому направлению. У постройки, вытянутой по одной оси, у угла оно
// больше, чем у стены, — так и должно быть, а дальность оружия мерится до
// поверхности, а не до середины габаритов.
func (e Entity) SurfaceGap(x, y float64) float64 {
	dx := e.Position.X - x
	dy := e.Position.Y - y
	dist := math.Hypot(dx, dy)
	if dist <= 1e-9 {
		return 0
	}
	hx, hy := e.Half[0], e.Half[1]
	if hx <= 0 {
		hx = e.Radius
	}
	if hy <= 0 {
		hy = e.Radius
	}
	extent := math.Inf(1)
	if ux := math.Abs(dx) / dist; ux > 1e-9 {
		extent = math.Min(extent, hx/ux)
	}
	if uy := math.Abs(dy) / dist; uy > 1e-9 {
		extent = math.Min(extent, hy/uy)
	}
	if math.IsInf(extent, 1) {
		extent = 0
	}
	if extent > dist {
		extent = dist
	}
	return dist - extent
}

func (e Entity) UnderConstruction() bool {
	return e.Build.Required > 0 && e.Build.Progress < e.Build.Required
}

// ConstructionRatio возвращает прогресс строительства в диапазоне [0, 1].
func (e Entity) ConstructionRatio() float64 {
	if e.Build.Required <= 0 {
		return 1
	}
	ratio := e.Build.Progress / e.Build.Required
	if ratio > 1 {
		return 1
	}
	if ratio < 0 {
		return 0
	}
	return ratio
}

var (
	ErrNotFound           = errors.New("entities: сущность не найдена")
	ErrNotMoving          = errors.New("entities: у сущности нет скорости — она не может двигаться")
	ErrNotAlive           = errors.New("entities: сущность мертва и не принимает приказы")
	ErrInvalidOrder       = errors.New("entities: некорректный приказ движения")
	ErrNotCapturable      = errors.New("entities: сущность нельзя захватить")
	ErrNotSelectable      = errors.New("entities: сущность нельзя выбрать")
	ErrIndestructible     = errors.New("entities: сущность нельзя уничтожить — только захватить")
	ErrPointLocked        = errors.New("entities: на точке стоит постройка владельца — сначала уничтожьте её")
	ErrInvalidBuilder     = errors.New("entities: недопустимый строитель")
	ErrNoConstruction     = errors.New("entities: у здания нет незавершённого строительства")
	ErrBuilderNotAssigned = errors.New("entities: строитель не назначен этому зданию")
	ErrPlayerNotFound     = errors.New("entities: игрок не зарегистрирован")
	ErrNoWeapon           = errors.New("entities: у сущности нет дальнобойного оружия")
	ErrProductionDenied   = errors.New("entities: этому игроку запрещено производство такого типа")
	ErrPlayerExists       = errors.New("entities: игрок уже зарегистрирован")
	ErrInvalidPlayer      = errors.New("entities: недопустимый игрок")
)

// Factory создаёт, размещает и хранит состояние сущностей игрового мира.
// Методы потокобезопасны; возвращаемые сущности — независимые снимки.
type Factory struct {
	mu       sync.RWMutex
	nextID   uint64
	entities map[string]Entity
	players  map[PlayerID]Player
}

// NewFactory создаёт пустую фабрику и реестр игроков.
func NewFactory() *Factory {
	return &Factory{
		entities: make(map[string]Entity),
		players:  make(map[PlayerID]Player),
	}
}

// RegisterPlayer добавляет игрока с явным списком разрешённых типов производства.
// Повторная регистрация того же ID возвращает ErrPlayerExists.
func (f *Factory) RegisterPlayer(player Player) error {
	if f == nil {
		return errors.New("entities: nil factory")
	}
	if player.ID == "" || player.ID == NeutralPlayer || validatePlayerID(player.ID) != nil {
		return ErrInvalidPlayer
	}
	if strings.TrimSpace(player.Name) == "" {
		return errors.New("entities: имя игрока обязательно")
	}
	allowed := make(map[Kind]bool, len(player.AllowedProduction))
	for _, kind := range player.AllowedProduction {
		if !kind.valid() {
			return fmt.Errorf("entities: недопустимый тип производства %d", kind)
		}
		allowed[kind] = true
	}
	player.AllowedProduction = make([]Kind, 0, len(allowed))
	for kind := KindUnit; kind <= KindGameplay; kind++ {
		if allowed[kind] {
			player.AllowedProduction = append(player.AllowedProduction, kind)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.players[player.ID]; exists {
		return ErrPlayerExists
	}
	f.players[player.ID] = clonePlayer(player)
	return nil
}

// SetProductionRules replaces the allowed production types of a registered player.
func (f *Factory) SetProductionRules(id PlayerID, allowed []Kind) error {
	if f == nil {
		return errors.New("entities: nil factory")
	}
	for _, kind := range allowed {
		if !kind.valid() {
			return fmt.Errorf("entities: недопустимый тип производства %d", kind)
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	player, ok := f.players[id]
	if !ok {
		return ErrPlayerNotFound
	}
	player.AllowedProduction = append([]Kind(nil), allowed...)
	player.AllowedProduction = uniqueKinds(player.AllowedProduction)
	f.players[id] = player
	return nil
}

// Player возвращает копию зарегистрированного игрока.
func (f *Factory) Player(id PlayerID) (Player, bool) {
	if f == nil {
		return Player{}, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	player, ok := f.players[id]
	return clonePlayer(player), ok
}

// CanProduce проверяет, зарегистрирован ли игрок и разрешён ли ему тип.
func (f *Factory) CanProduce(playerID PlayerID, kind Kind) bool {
	if f == nil || !kind.valid() {
		return false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	player, ok := f.players[playerID]
	if !ok {
		return false
	}
	for _, allowed := range player.AllowedProduction {
		if allowed == kind {
			return true
		}
	}
	return false
}

// Spawn создаёт сущность и размещает её на карте. Если Spec.Owner не задан,
// игрок-производитель становится владельцем. Явный NeutralPlayer сохраняет
// нейтрального владельца; пустой Owner и ProducedBy оставляют его неназначенным.
func (f *Factory) Spawn(spec Spec, position Position) (Entity, error) {
	if f == nil {
		return Entity{}, errors.New("entities: nil factory")
	}
	if strings.TrimSpace(spec.Name) == "" {
		return Entity{}, errors.New("entities: имя сущности обязательно")
	}
	if !spec.Kind.valid() {
		return Entity{}, fmt.Errorf("entities: недопустимый тип %d", spec.Kind)
	}
	if !finite(position.X) || !finite(position.Y) {
		return Entity{}, errors.New("entities: координаты должны быть конечными числами")
	}
	if err := validatePlayerID(spec.ProducedBy); err != nil {
		return Entity{}, fmt.Errorf("entities: производитель: %w", err)
	}
	if err := validatePlayerID(spec.Owner); err != nil {
		return Entity{}, fmt.Errorf("entities: владелец: %w", err)
	}
	if !finite(spec.ConstructionWork) || spec.ConstructionWork < 0 {
		return Entity{}, errors.New("entities: объём строительства должен быть конечным и неотрицательным")
	}
	if spec.SelectionUI.DisplayType < 0 || spec.SelectionUI.DisplayType > 1 {
		return Entity{}, errors.New("entities: Selected UI display type должен быть 0 (circle) или 1 (square)")
	}
	for _, value := range append(spec.SelectionUI.Scale[:], spec.SelectionUI.Offset[:]...) {
		if !finite(value) {
			return Entity{}, errors.New("entities: Selected UI scale и offset должны быть конечными числами")
		}
	}
	if spec.ConstructionWork > 0 && spec.Kind != KindBuilding {
		return Entity{}, errors.New("entities: строительство доступно только для зданий")
	}

	owner := spec.Owner
	if owner == "" {
		owner = spec.ProducedBy
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if spec.ProducedBy != "" {
		player, ok := f.players[spec.ProducedBy]
		if !ok {
			return Entity{}, fmt.Errorf("%w: %s", ErrPlayerNotFound, spec.ProducedBy)
		}
		if !playerCanProduce(player, spec.Kind) {
			return Entity{}, fmt.Errorf("%w: %s не может создать %v", ErrProductionDenied, spec.ProducedBy, spec.Kind)
		}
	}
	if owner != "" && owner != NeutralPlayer {
		if _, ok := f.players[owner]; !ok {
			return Entity{}, fmt.Errorf("%w: %s", ErrPlayerNotFound, owner)
		}
	}
	f.nextID++
	id := fmt.Sprintf("entity-%08d", f.nextID)
	// Радиус без данных — метр: по нему считается дистанция до цели, и
	// нулевой радиус свёл бы остановку на атаке к дистанции оружия.
	radius := spec.Radius
	if radius <= 0 || math.IsNaN(radius) || math.IsInf(radius, 0) {
		radius = 1
	}
	// Основание — бокс: где не заданы полуоси, его даёт радиус.
	half := spec.Half
	if half[0] <= 0 || math.IsNaN(half[0]) || math.IsInf(half[0], 0) {
		half[0] = radius
	}
	if half[1] <= 0 || math.IsNaN(half[1]) || math.IsInf(half[1], 0) {
		half[1] = radius
	}
	// Восстановление после уничтожения: шанс в долях, доля здоровья в
	// долях от максимума, задержка в секундах. Без данных тело не встаёт.
	getUpChance := spec.GetUpChance
	if getUpChance < 0 || math.IsNaN(getUpChance) {
		getUpChance = 0
	} else if getUpChance > 1 {
		getUpChance = 1
	}
	getUpPercent := spec.GetUpHealthPercent
	if getUpPercent <= 0 || getUpPercent > 1 || math.IsNaN(getUpPercent) {
		getUpPercent = 0.3
	}
	getUpTime := spec.GetUpTime
	if getUpTime <= 0 || math.IsNaN(getUpTime) {
		getUpTime = 6
	}
	// Броня 100 — базовая у всех юнитов и построек DoWDE; нулевая означает,
	// что чертёж её не дал.
	armour := spec.Armour
	if armour <= 0 {
		armour = 100
	}
	health := Health{Current: spec.HitPoints, Max: spec.HitPoints}
	if spec.Dead {
		health.Current = 0
	}
	// Стройплощадка появляется почти без здоровья: оно растёт вместе с
	// прогрессом работы строителя (см. Contribute). Долю задаёт чертёж расы
	// (tuning/building_info/start_health_percent), а без неё —
	// ConstructionStartHealth.
	startHealth := 0.0
	if spec.ConstructionWork > 0 && !spec.Dead {
		startHealth = spec.StartHealthRatio
		if startHealth <= 0 || startHealth > 1 {
			startHealth = ConstructionStartHealth
		}
		if health.Max > 0 {
			health.Current = health.Max * startHealth
		}
	}
	entity := Entity{
		ID: id, Name: spec.Name, Kind: spec.Kind,
		ProducedBy: spec.ProducedBy, Owner: owner,
		State: State{
			Position:    position,
			Speed:       spec.Speed,
			AccelTime:   spec.AccelTime,
			BrakeTime:   spec.BrakeTime,
			Alive:       !spec.Dead,
			Health:      health,
			Morale:      spec.MoraleMax,
			MoraleMax:   spec.MoraleMax,
			OrderStatus: OrderNone,
			// Автоатака у всех включена: выключать её будет отдельный приказ.
			AutoAttack: !spec.DisableAutoAttack,
		},
		Selectable: spec.Selectable, Capturable: spec.Capturable,
		Indestructible: spec.Indestructible, BuiltOn: spec.BuiltOn,
		SquadID:            spec.SquadID,
		Weapon:             spec.Weapon,
		Radius:             radius,
		Half:               half,
		GetUpChance:        getUpChance,
		GetUpHealthPercent: getUpPercent,
		GetUpTime:          getUpTime,
		UsableBody:         spec.UsableBody && spec.Kind == KindUnit,
		FromBuilding:       spec.FromBuilding,
		ArmourClass:        spec.ArmourClass,
		Armour:             armour,
		SelectionUI:        spec.SelectionUI,
		Build:              Construction{Required: spec.ConstructionWork, StartHealth: startHealth},
		Income:             spec.Income,
		CapUsage:           spec.CapUsage,
		CapBonus:           spec.CapBonus,
	}
	f.entities[id] = entity
	// Здание, поставленное на точку захвата (BuiltOn), забирает точку себе:
	// так слаг-депозит захватывается термогенератором. Если точка уже
	// принадлежит построившему — ничего не меняется.
	if spec.BuiltOn != "" && owner != "" && owner != NeutralPlayer {
		if point, ok := f.entities[spec.BuiltOn]; ok && point.Capturable && point.Owner != owner {
			f.assignOwnerLocked(&point, owner)
			f.entities[spec.BuiltOn] = point
		}
	}
	return cloneEntity(entity), nil
}

// AssignOwner назначает владельца, снимает его (owner="") или назначает
// нейтрального владельца. Это также используется для игровых объектов,
// которым владелец назначается позднее.
func (f *Factory) AssignOwner(id string, owner PlayerID) error {
	if err := validatePlayerID(owner); err != nil {
		return fmt.Errorf("entities: владелец: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if owner != "" && owner != NeutralPlayer {
		if _, ok := f.players[owner]; !ok {
			return fmt.Errorf("%w: %s", ErrPlayerNotFound, owner)
		}
	}
	f.assignOwnerLocked(&entity, owner)
	f.entities[id] = entity
	return nil
}

// Capture передаёт захватываемую сущность игроку. Нейтральный или пустой ID
// не считается действием захвата — для снятия владельца используйте AssignOwner.
//
// Пока на захваченной точке стоит постройка её владельца, снять с неё флаг
// нельзя: точка переходит другому игроку только после уничтожения постройки.
// Нейтральная и пустая точка захватываются свободно.
func (f *Factory) Capture(id string, player PlayerID) error {
	if f == nil {
		return errors.New("entities: nil factory")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.captureLocked(id, player)
}

// captureLocked — ядро Capture без взятия мьютекса: оно нужно и движению
// (см. state.go), которое приходит на точку уже под взятым мьютексом.
func (f *Factory) captureLocked(id string, player PlayerID) error {
	if err := f.captureReadyLocked(id, player); err != nil {
		return err
	}
	entity := f.entities[id]
	f.assignOwnerLocked(&entity, player)
	// Флаг уже поднят: прогресс и его хозяин совпадают с владельцем.
	entity.CaptureProgress = 1
	entity.CapturePlayer = player
	f.entities[id] = entity
	return nil
}

// captureReadyLocked — та же проверка, что и в captureLocked, но без
// смены владельца: ею проверяется точка в момент прибытия бойца, чтобы
// заведомо заблокированную постройкой точку захват не начинал вовсе.
func (f *Factory) captureReadyLocked(id string, player PlayerID) error {
	if player == "" || player == NeutralPlayer {
		return errors.New("entities: захватить может только именованный игрок")
	}
	if err := validatePlayerID(player); err != nil {
		return fmt.Errorf("entities: захватчик: %w", err)
	}
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if _, ok := f.players[player]; !ok {
		return fmt.Errorf("%w: %s", ErrPlayerNotFound, player)
	}
	if !entity.Capturable {
		return ErrNotCapturable
	}
	if entity.Owner != "" && entity.Owner != NeutralPlayer && entity.Owner != player && f.pointLocked(id, player) {
		return ErrPointLocked
	}
	return nil
}

// pointLocked — стоит ли на точке постройка, не принадлежащая захватчику:
// такая постройка блокирует снятие флага, пока её не уничтожат.
// Вызывается с захваченным мьютексом.
func (f *Factory) pointLocked(pointID string, player PlayerID) bool {
	for _, other := range f.entities {
		if other.BuiltOn == pointID && other.Owner != player {
			return true
		}
	}
	return false
}

// Destroy уничтожает сущность. Точки захвата (Indestructible) уничтожить
// нельзя — они, по правилам игры, только захватываются; уничтожение постройки
// на точке снимает с точки блокировку (см. Capture).
func (f *Factory) Destroy(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if entity.Indestructible {
		return ErrIndestructible
	}
	delete(f.entities, id)
	// Убрать погибшего строителя из незавершённых строительств.
	for otherID, other := range f.entities {
		kept := other.Build.Builders[:0]
		for _, builderID := range other.Build.Builders {
			if builderID != id {
				kept = append(kept, builderID)
			}
		}
		if len(kept) != len(other.Build.Builders) {
			other.Build.Builders = kept
			f.entities[otherID] = other
		}
	}
	return nil
}

// Kill уничтожает сущность по приказу её владельца (клавиша Delete):
// обычная гибель — с броском шанса восстановления у некронов и со всеми
// последствиями, которые сцена разбирает по Alive=false.
func (f *Factory) Kill(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if !entity.Alive {
		return ErrNotAlive
	}
	f.killLocked(id)
	return nil
}

// Revive поднимает лежащее тело в строй с долей здоровья healthFraction:
// так работают воскрешение лорда и найм, который занимает место своего
// стартового тела. Тело оживает на том же месте и с тем же ID — новый юнит
// рядом не появляется. Живой юнит возвращается как есть.
func (f *Factory) Revive(id string, healthFraction float64) (Entity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return Entity{}, ErrNotFound
	}
	if entity.Alive {
		return entity, nil
	}
	if healthFraction <= 0 || healthFraction > 1 || math.IsNaN(healthFraction) {
		healthFraction = 0.3
	}
	health := entity.Health.Max * healthFraction
	if health < 1 {
		health = 1
	}
	entity.Alive = true
	entity.Health.Current = health
	entity.ReviveIn = 0
	entity.Selectable = true
	entity.CorpseTaken = true
	entity.Order = nil
	entity.OrderStatus = OrderNone
	entity.Attacking = false
	entity.Capturing = false
	entity.CaptureProgress = 0
	entity.Cooldown = 0
	entity.Velocity = 0
	f.entities[id] = entity
	return entity, nil
}

// TakeCorpse убирает тело с карты: его собрал томб-спайдер. Тело остаётся
// в реестре мёртвым и помеченным CorpseTaken — второй раз оно не достанется
// ни другому спайдеру, ни воскрешению.
func (f *Factory) TakeCorpse(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	if entity.Alive || entity.CorpseTaken {
		return ErrNotAlive
	}
	entity.CorpseTaken = true
	f.entities[id] = entity
	return nil
}

// SetDormant усыпляет или будит постройку: спящая занята пробудившимся
// юнитом — её не выбирают, не атакуют, она не работает до возвращения.
func (f *Factory) SetDormant(id string, dormant bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	entity.Dormant = dormant
	f.entities[id] = entity
	return nil
}

// SetSelectable меняет, разрешено ли выбирать сущность в игровом интерфейсе.
func (f *Factory) SetSelectable(id string, selectable bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	entity, ok := f.entities[id]
	if !ok {
		return ErrNotFound
	}
	entity.Selectable = selectable
	f.entities[id] = entity
	return nil
}

// CanSelect возвращает true только для зарегистрированных выбираемых сущностей.
func (f *Factory) CanSelect(id string) bool {
	entity, ok := f.Get(id)
	return ok && entity.Selectable
}

// SelectAt выбирает ближайшую сущность в заданном радиусе от позиции на карте.
// Любой владелец (включая противника и NeutralPlayer) может быть выбран.
func (f *Factory) SelectAt(position Position, radius float64) (Entity, error) {
	if f == nil || !finite(position.X) || !finite(position.Y) || !finite(radius) || radius < 0 {
		return Entity{}, ErrNotSelectable
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	bestDistance := radius * radius
	var selected Entity
	found := false
	for _, entity := range f.entities {
		if !entity.Selectable {
			continue
		}
		dx, dy := entity.Position.X-position.X, entity.Position.Y-position.Y
		distance := dx*dx + dy*dy
		if distance <= radius*radius && (!found || distance < bestDistance || (distance == bestDistance && entity.ID < selected.ID)) {
			bestDistance = distance
			selected = entity
			found = true
		}
	}
	if !found {
		return Entity{}, ErrNotFound
	}
	return cloneEntity(selected), nil
}

// Select возвращает выбираемую сущность либо ErrNotSelectable.
func (f *Factory) Select(id string) (Entity, error) {
	entity, ok := f.Get(id)
	if !ok {
		return Entity{}, ErrNotFound
	}
	if !entity.Selectable {
		return Entity{}, ErrNotSelectable
	}
	return entity, nil
}

// AddBuilder назначает принадлежащий тому же игроку юнит строителем здания.
func (f *Factory) AddBuilder(buildingID, builderID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	building, ok := f.entities[buildingID]
	if !ok {
		return ErrNotFound
	}
	builder, ok := f.entities[builderID]
	if !ok {
		return ErrNotFound
	}
	if building.Kind != KindBuilding || !building.UnderConstruction() ||
		builder.Kind != KindUnit || buildingID == builderID || building.Owner == "" ||
		building.Owner == NeutralPlayer || builder.Owner != building.Owner {
		return ErrInvalidBuilder
	}
	for _, existing := range building.Build.Builders {
		if existing == builderID {
			return nil
		}
	}
	building.Build.Builders = append(building.Build.Builders, builderID)
	f.entities[buildingID] = building
	return nil
}

// RemoveBuilder снимает назначенного строителя с незавершённого здания.
func (f *Factory) RemoveBuilder(buildingID, builderID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	building, ok := f.entities[buildingID]
	if !ok {
		return ErrNotFound
	}
	for i, id := range building.Build.Builders {
		if id == builderID {
			building.Build.Builders = append(building.Build.Builders[:i], building.Build.Builders[i+1:]...)
			f.entities[buildingID] = building
			return nil
		}
	}
	return ErrBuilderNotAssigned
}

// Contribute добавляет строительную работу от назначенного юнита и возвращает
// обновлённое значение прогресса. По завершении строительства строители снимаются.
func (f *Factory) Contribute(buildingID, builderID string, work float64) (float64, error) {
	if !finite(work) || work <= 0 {
		return 0, errors.New("entities: строительная работа должна быть положительной и конечной")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	building, ok := f.entities[buildingID]
	if !ok {
		return 0, ErrNotFound
	}
	if !building.UnderConstruction() {
		return building.Build.Progress, ErrNoConstruction
	}
	assigned := false
	for _, id := range building.Build.Builders {
		if id == builderID {
			assigned = true
			break
		}
	}
	if !assigned {
		return building.Build.Progress, ErrBuilderNotAssigned
	}
	builder, ok := f.entities[builderID]
	if !ok || builder.Kind != KindUnit || builder.Owner != building.Owner {
		return building.Build.Progress, ErrInvalidBuilder
	}
	building.Build.Progress = math.Min(building.Build.Required, building.Build.Progress+work)
	if building.Health.Max > 0 {
		if building.UnderConstruction() {
			// Здоровье идёт к максимуму вместе с прогрессом: на 100% работы
			// у постройки уже полный запас. Стартовая доля — та, с которой
			// площадку заложили (tuning/building_info).
			start := building.Build.StartHealth
			if start <= 0 || start > 1 {
				start = ConstructionStartHealth
			}
			ratio := building.ConstructionRatio()
			building.Health.Current = building.Health.Max * (start + (1-start)*ratio)
		} else {
			building.Health.Current = building.Health.Max
		}
	}
	if !building.UnderConstruction() {
		building.Build.Builders = nil
	}
	f.entities[buildingID] = building
	return building.Build.Progress, nil
}

// Get возвращает снимок сущности.
func (f *Factory) Get(id string) (Entity, bool) {
	if f == nil {
		return Entity{}, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	entity, ok := f.entities[id]
	return cloneEntity(entity), ok
}

// All возвращает снимки всех сущностей в стабильном порядке ID.
func (f *Factory) All() []Entity {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	ids := make([]string, 0, len(f.entities))
	for id := range f.entities {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	entities := make([]Entity, 0, len(ids))
	for _, id := range ids {
		entities = append(entities, cloneEntity(f.entities[id]))
	}
	return entities
}

func (f *Factory) assignOwnerLocked(entity *Entity, owner PlayerID) {
	entity.Owner = owner
	builders := entity.Build.Builders[:0]
	for _, id := range entity.Build.Builders {
		builder, ok := f.entities[id]
		if ok && owner != "" && owner != NeutralPlayer && builder.Owner == owner {
			builders = append(builders, id)
		}
	}
	entity.Build.Builders = builders
}

func validatePlayerID(id PlayerID) error {
	if id != "" && strings.TrimSpace(string(id)) != string(id) {
		return errors.New("ID игрока не должен содержать крайние пробелы")
	}
	return nil
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func cloneEntity(entity Entity) Entity {
	entity.Build.Builders = append([]string(nil), entity.Build.Builders...)
	return entity
}

func clonePlayer(player Player) Player {
	player.AllowedProduction = append([]Kind(nil), player.AllowedProduction...)
	return player
}

func playerCanProduce(player Player, kind Kind) bool {
	for _, allowed := range player.AllowedProduction {
		if allowed == kind {
			return true
		}
	}
	return false
}

func uniqueKinds(kinds []Kind) []Kind {
	seen := make(map[Kind]bool, len(kinds))
	unique := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if !seen[kind] {
			seen[kind] = true
			unique = append(unique, kind)
		}
	}
	return unique
}
