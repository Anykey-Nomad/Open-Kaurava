package scenes

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"

	"dow-mod-studio/racebps"
	"dow-mod-studio/sgbview"
)

// PlayerSetup — выбор игроков перед началом игры.
//
// Экран «Выбор игроков» его заполняет, 3D-предпросмотр читает: по нему
// известно, какая раса у какого слота и какие постройки ставить на
// стартовые позиции.
type PlayerSetup struct {
	// Slots — слоты сценария: у каждого своя стартовая позиция.
	Slots []sgbview.PlayerSlot
	// Races — играбельные расы модуля.
	Races []racebps.Race
	// Pick — выбор расы по номеру слота: индекс в Races,
	// -1 — раса случайная.
	Pick map[int]int
	// Resolved — раса каждого слота после разбора случайных выборов.
	Resolved map[int]int
	// resolvedPick — выбор, для которого посчитан Resolved. Он нужен,
	// чтобы случайная раса не выпадала заново при возврате на экран
	// выбора игроков.
	resolvedPick map[int]int
	// Own — номер слота, за который играет пользователь.
	Own int
	// Builds — стартовые постройки по слотам.
	Builds []sgbview.SlotBuild
}

// Setup — текущий выбор игроков. Общее состояние: его пишет экран
// выбора игроков и читает 3D-предпросмотр.
var Setup PlayerSetup

// RaceIndex — индекс выбранной расы слота: -1, если раса случайная.
func (s *PlayerSetup) RaceIndex(slot int) int {
	if s.Pick == nil {
		return -1
	}
	i, ok := s.Pick[slot]
	if !ok {
		return -1
	}
	return i
}

// RaceLabel — название расы слота: выбранная, а если выбор случайный —
// та, что выпала при разборе.
func (s *PlayerSetup) RaceLabel(slot int) string {
	if i, ok := s.Resolved[slot]; ok && i >= 0 && i < len(s.Races) {
		return s.Races[i].Label()
	}
	if i := s.RaceIndex(slot); i >= 0 && i < len(s.Races) {
		return s.Races[i].Label()
	}
	return "случайная"
}

// SortedSlots — слоты по номеру игрока.
func (s *PlayerSetup) SortedSlots() []sgbview.PlayerSlot {
	slots := append([]sgbview.PlayerSlot(nil), s.Slots...)
	sort.Slice(slots, func(i, j int) bool { return slots[i].Owner < slots[j].Owner })
	return slots
}

// Resolve назначает случайные расы тем слотам, у которых выбор не задан,
// и собирает список стартовых построек.
//
// Случайные расы не повторяются: двум игрокам одна и та же раса не
// достаётся, если есть из чего выбрать. Уже выпавшая случайная раса
// сохраняется, пока игрок её не сменил: иначе раса менялась бы при
// каждом возврате на экран выбора.
func (s *PlayerSetup) Resolve() {
	slots := s.SortedSlots()

	resolved := map[int]int{}
	used := map[int]bool{}
	// Слоты, которым раса достаётся случайно.
	var random []sgbview.PlayerSlot

	for _, slot := range slots {
		i := s.RaceIndex(slot.Owner)
		if i >= 0 && i < len(s.Races) {
			resolved[slot.Owner] = i
			used[i] = true
			continue
		}
		random = append(random, slot)
	}

	// Прежние случайные расы: оставляем, если выбор игрока не менялся.
	var pending []sgbview.PlayerSlot
	for _, slot := range random {
		if s.resolvedPick == nil || s.resolvedPick[slot.Owner] != -1 {
			pending = append(pending, slot)
			continue
		}
		old, ok := s.Resolved[slot.Owner]
		if !ok || old < 0 || old >= len(s.Races) || used[old] {
			pending = append(pending, slot)
			continue
		}
		resolved[slot.Owner] = old
		used[old] = true
	}

	// Свободные расы: те, что не заняты выбором игрока.
	free := make([]int, 0, len(s.Races))
	for i := range s.Races {
		if !used[i] {
			free = append(free, i)
		}
	}
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(free), func(i, j int) { free[i], free[j] = free[j], free[i] })

	next := 0
	for _, slot := range pending {
		switch {
		case len(s.Races) == 0:
		case next < len(free):
			resolved[slot.Owner] = free[next]
			next++
		default:
			resolved[slot.Owner] = rnd.Intn(len(s.Races))
		}
	}

	s.Resolved = resolved
	s.resolvedPick = map[int]int{}
	for _, slot := range slots {
		s.resolvedPick[slot.Owner] = s.RaceIndex(slot.Owner)
	}

	// Стартовые постройки расы: все, что перечислены в её чертеже.
	s.Builds = nil
	for _, slot := range slots {
		i, ok := resolved[slot.Owner]
		if !ok || i < 0 || i >= len(s.Races) {
			continue
		}
		race := s.Races[i]
		for _, building := range race.StartingBuildings {
			path := race.StructurePath(building)
			if path == "" {
				continue
			}
			s.Builds = append(s.Builds, sgbview.SlotBuild{Slot: slot.Owner, Blueprint: path})
		}
	}
}

// Key — подпись выбора: по ней видно, что выбор изменился и сцену
// надо перезагрузить.
func (s *PlayerSetup) Key() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(s.Own))
	for _, slot := range s.SortedSlots() {
		fmt.Fprintf(&b, "|%d:%d", slot.Owner, s.RaceIndex(slot.Owner))
		if i, ok := s.Resolved[slot.Owner]; ok {
			fmt.Fprintf(&b, "=%d", i)
		}
	}
	return b.String()
}
